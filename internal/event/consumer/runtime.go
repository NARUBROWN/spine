package consumer

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"sync"
	"time"

	eventadapter "github.com/NARUBROWN/spine/contract/event"
	"github.com/NARUBROWN/spine/internal/configuration"
	"github.com/NARUBROWN/spine/internal/event/cleanup"
	"github.com/NARUBROWN/spine/internal/pipeline"
)

type runnerFactory interface {
	Build(reg Registration) (Reader, error)
}

type contextRunnerFactory interface {
	BuildContext(context.Context, Registration) (Reader, error)
}

type Runtime struct {
	registry       *Registry
	factory        runnerFactory
	pipeline       *pipeline.Pipeline
	lifecycleMu    sync.Mutex
	prepared       map[string]Reader
	cleanupMu      sync.Mutex
	cleanupOnce    sync.Once
	cleanupErr     error
	cleanupHistory cleanup.History
	workers        sync.WaitGroup
	started        bool
	stopped        bool
	stopOnce       sync.Once
	doneOnce       sync.Once
	cancel         context.CancelFunc
	errChan        chan error
	done           chan struct{}
	waitRetry      func(context.Context, time.Duration) bool
	jitterDelay    func(time.Duration, float64) time.Duration
}

const (
	consumerReadRetryInitialDelay = 100 * time.Millisecond
	consumerReadRetryMaxDelay     = 5 * time.Second
)

// TransportRetryPolicy는 브로커 전송 오류가 발생한 뒤 Reader를 다시 생성하는 방식을 제어합니다.
// 핸들러 실패는 이 정책과 별개로 메시지 ACK/NACK에 따라 처리됩니다.
type TransportRetryPolicy struct {
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
	Jitter       float64
	MaxAttempts  int
}

type retryPolicyProvider interface {
	ConsumerRetryPolicy() TransportRetryPolicy
}

type startupValidator interface {
	ValidateStartup(context.Context, Registration) error
}

func NewTransportRetryPolicy(initialDelay, maxDelay time.Duration, multiplier, jitter float64, maxAttempts int) TransportRetryPolicy {
	policy := TransportRetryPolicy{
		InitialDelay: initialDelay,
		MaxDelay:     maxDelay,
		Multiplier:   multiplier,
		Jitter:       jitter,
		MaxAttempts:  maxAttempts,
	}
	if policy.InitialDelay == 0 {
		policy.InitialDelay = consumerReadRetryInitialDelay
	}
	if policy.MaxDelay == 0 {
		policy.MaxDelay = consumerReadRetryMaxDelay
	}
	if policy.Multiplier == 0 {
		policy.Multiplier = 2
	}
	if policy.Jitter == 0 {
		policy.Jitter = 0.2
	}
	return policy
}

func NewRuntime(registry *Registry, factory runnerFactory, pipeline *pipeline.Pipeline, history ...configuration.CleanupErrorHistoryOptions) *Runtime {
	if registry == nil {
		panic("consumer: registry cannot be nil")
	}
	if factory == nil {
		panic("consumer: factory cannot be nil")
	}
	if pipeline == nil {
		panic("consumer: pipeline cannot be nil")
	}

	return &Runtime{
		registry:       registry,
		factory:        factory,
		pipeline:       pipeline,
		errChan:        make(chan error, max(1, len(registry.Registrations()))),
		done:           make(chan struct{}),
		waitRetry:      waitForRetry,
		jitterDelay:    jitteredDelay,
		cleanupHistory: cleanup.NewHistory(history...),
	}
}

func (r *Runtime) retryPolicy() TransportRetryPolicy {
	if provider, ok := r.factory.(retryPolicyProvider); ok {
		return provider.ConsumerRetryPolicy()
	}
	return NewTransportRetryPolicy(0, 0, 0, 0, 0)
}

// Errors는 런타임 내부에서 발생한 치명적 에러를 전달받기 위한 채널입니다.
// 치명적 에러로 중단할 때는 에러를 먼저 채널에 기록한 뒤 Done을 닫습니다.
// Errors와 Done을 함께 기다리는 호출자는 Done을 관찰한 경우 Errors를 한 번 더
// 확인해야 합니다. 채널은 close되지 않습니다.
func (r *Runtime) Errors() <-chan error {
	return r.errChan
}

func (r *Runtime) Done() <-chan struct{} {
	return r.done
}

func (r *Runtime) Start(ctx context.Context) {
	r.lifecycleMu.Lock()
	if r.started || r.stopped {
		r.lifecycleMu.Unlock()
		return
	}
	ctx, r.cancel = context.WithCancel(ctx)
	r.started = true
	registrations := r.registry.Registrations()
	r.workers.Add(len(registrations))
	for _, registration := range registrations {
		log.Printf("[Event Consumer] Starting consumer for topic '%s'", registration.Topic)
		initial := r.prepared[registration.Topic]
		delete(r.prepared, registration.Topic)
		go func(reg Registration, initial Reader) {
			defer r.workers.Done()
			reader := initial
			defer func() {
				if reader != nil {
					_ = r.closeReader(reader)
				}
			}()
			select {
			case <-ctx.Done():
				return
			default:
			}

			retryPolicy := r.retryPolicy()
			if reader == nil {
				var ok bool
				reader, ok = r.acquireReader(ctx, reg, retryPolicy, nil, true, "initialization")
				if !ok {
					return
				}
			}
			var ok bool

			for {
				select {
				case <-ctx.Done():
					return
				default:
					msg, err := reader.Read(ctx)
					if err != nil {
						if ctx.Err() != nil {
							return
						}
						log.Printf("[Event Consumer] Transport read failed (topic=%s): %v", reg.Topic, err)
						_ = r.closeReader(reader)
						reader = nil
						reader, ok = r.acquireReader(ctx, reg, retryPolicy, err, false, "reconnect")
						if !ok {
							return
						}
						continue
					}

					// 컨슈머 실행 컨텍스트 생성
					reqCtx := NewRequestContext(ctx, msg, nil)

					// 핸들러 실행
					if err := r.pipeline.Execute(reqCtx); err != nil {
						log.Printf(
							"[Event Consumer] Handler execution failed (%s): %v",
							reg.Topic,
							err,
						)
						// 핸들러 실패 시 NACK
						if nackErr := msg.Nack(); nackErr != nil {
							if !errors.Is(nackErr, ErrReaderInvalidated) {
								log.Printf(
									"[Event Consumer] NACK failed (%s): %v",
									reg.Topic,
									nackErr,
								)
							}
							_ = r.closeReader(reader)
							reader = nil
							// 실패한 전달을 다시 읽기 위해 reader를 폐기한 경우에도 즉시
							// 재생성하지 않습니다. 영구적으로 실패하는 메시지가 consumer group
							// 재가입과 같은 offset 재처리를 무제한으로 빠르게 반복하지 않도록
							// 전송 재시도 정책의 backoff를 먼저 적용합니다.
							reader, ok = r.acquireReader(ctx, reg, retryPolicy, nackErr, false, "reconnect")
							if !ok {
								return
							}
						}
						continue
					}

					// 핸들러 성공 시 ACK
					if ackErr := msg.Ack(); ackErr != nil {
						log.Printf(
							"[Event Consumer] ACK failed (%s): %v",
							reg.Topic,
							ackErr,
						)
						_ = r.closeReader(reader)
						reader = nil
						// ACK 실패도 같은 offset을 다시 읽게 되므로 즉시 재가입하지 않고
						// 동일한 backoff 경계를 적용합니다.
						reader, ok = r.acquireReader(ctx, reg, retryPolicy, ackErr, false, "reconnect")
						if !ok {
							return
						}
					}
				}
			}
		}(registration, initial)
	}
	r.lifecycleMu.Unlock()
	go func() {
		r.workers.Wait()
		r.finish()
	}()
}

func (r *Runtime) acquireReader(
	ctx context.Context,
	reg Registration,
	policy TransportRetryPolicy,
	lastErr error,
	buildImmediately bool,
	stage string,
) (Reader, bool) {
	select {
	case <-ctx.Done():
		return nil, false
	default:
	}
	if buildImmediately {
		reader, err := r.buildReader(ctx, reg)
		if err == nil {
			return reader, true
		}
		if permanentFactoryError(err) {
			r.forwardError(err)
			r.initiateStop()
			return nil, false
		}
		lastErr = err
		log.Printf("[Event Consumer] Transport %s failed (topic=%s): %v", stage, reg.Topic, err)
	}

	delay := policy.InitialDelay
	for attempts := 1; ; attempts++ {
		if policy.MaxAttempts > 0 && attempts > policy.MaxAttempts {
			runtimeErr := fmt.Errorf("[Event Consumer] Transport %s attempts exhausted (topic=%s attempts=%d): %w", stage, reg.Topic, policy.MaxAttempts, lastErr)
			r.forwardError(runtimeErr)
			r.initiateStop()
			return nil, false
		}
		if !r.waitRetry(ctx, r.jitterDelay(delay, policy.Jitter)) {
			return nil, false
		}
		reader, err := r.buildReader(ctx, reg)
		delay = nextRetryDelay(delay, policy)
		if err == nil {
			return reader, true
		}
		if permanentFactoryError(err) {
			r.forwardError(err)
			r.initiateStop()
			return nil, false
		}
		lastErr = err
		log.Printf("[Event Consumer] Transport %s failed (topic=%s attempt=%d): %v", stage, reg.Topic, attempts, err)
	}
}

func (r *Runtime) buildReader(ctx context.Context, reg Registration) (Reader, error) {
	var reader Reader
	var err error
	if factory, ok := r.factory.(contextRunnerFactory); ok {
		reader, err = factory.BuildContext(ctx, reg)
	} else {
		reader, err = r.factory.Build(reg)
	}
	if err == nil && reader == nil {
		err = errors.New("consumer factory returned a nil reader")
	}
	if err != nil && reader != nil {
		_ = r.closeReader(reader)
	}
	return reader, err
}

func (r *Runtime) forwardError(err error) {
	select {
	case r.errChan <- err:
	default:
		log.Printf("%v (could not forward because the error channel is full)", err)
	}
}

func nextRetryDelay(delay time.Duration, policy TransportRetryPolicy) time.Duration {
	if float64(delay)*policy.Multiplier >= float64(policy.MaxDelay) {
		return policy.MaxDelay
	}
	return time.Duration(float64(delay) * policy.Multiplier)
}

func jitteredDelay(delay time.Duration, jitter float64) time.Duration {
	if jitter <= 0 {
		return delay
	}
	factor := 1 - jitter + rand.Float64()*(2*jitter)
	return max(0, time.Duration(float64(delay)*factor))
}

func waitForRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (r *Runtime) Validate() error {
	for _, reg := range r.registry.Registrations() {
		reader, err := r.buildValidationReader(context.Background(), reg)
		if err != nil {
			return fmt.Errorf("Consumer initialization failed (%s): %w", reg.Topic, err)
		}
		if err := reader.Close(); err != nil {
			return fmt.Errorf("Consumer shutdown failed (%s): %w", reg.Topic, err)
		}
	}
	return nil
}

// ValidateWithRetry verifies that every configured reader can be created before
// an ingress listener is exposed. Unlike Validate, transient build failures use
// the configured ConsumerRetry policy and can be interrupted by ctx.
func (r *Runtime) ValidateWithRetry(ctx context.Context) error {
	return r.validateWithRetry(ctx, false)
}

// PrepareWithRetry는 준비한 Reader를 보관하며 Start가 같은 연결을 이어받습니다.
func (r *Runtime) PrepareWithRetry(ctx context.Context) error { return r.validateWithRetry(ctx, true) }

func (r *Runtime) validateWithRetry(ctx context.Context, retain bool) (resultErr error) {
	if retain {
		r.lifecycleMu.Lock()
		defer r.lifecycleMu.Unlock()
		if r.started || r.stopped || len(r.prepared) > 0 {
			return eventadapter.ErrInvalidState
		}
	}
	acquired := make(map[string]Reader)
	defer func() {
		if resultErr != nil {
			for _, reader := range acquired {
				resultErr = errors.Join(resultErr, r.closeReader(reader))
			}
		}
	}()
	if ctx == nil {
		ctx = context.Background()
	}
	policy := r.retryPolicy()
	for _, reg := range r.registry.Registrations() {
		reader, err := r.buildValidationReader(ctx, reg)
		delay := policy.InitialDelay
		for retries := 1; err != nil; retries++ {
			if permanentFactoryError(err) {
				return err
			}
			if policy.MaxAttempts > 0 && retries > policy.MaxAttempts {
				return fmt.Errorf("Consumer initialization retries exhausted (%s retries=%d): %w", reg.Topic, policy.MaxAttempts, err)
			}
			if !r.waitRetry(ctx, r.jitterDelay(delay, policy.Jitter)) {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return fmt.Errorf("Consumer initialization canceled (%s): %w", reg.Topic, ctxErr)
				}
				return fmt.Errorf("Consumer initialization retry interrupted (%s): %w", reg.Topic, err)
			}
			reader, err = r.buildValidationReader(ctx, reg)
			delay = nextRetryDelay(delay, policy)
		}
		if retain {
			acquired[reg.Topic] = reader
		} else if err := r.closeReader(reader); err != nil {
			return fmt.Errorf("Consumer shutdown failed (%s): %w", reg.Topic, err)
		}
	}
	if retain {
		r.prepared = acquired
	}
	return nil
}

func (r *Runtime) buildValidationReader(ctx context.Context, reg Registration) (Reader, error) {
	if validator, ok := r.factory.(startupValidator); ok {
		if err := validator.ValidateStartup(ctx, reg); err != nil {
			return nil, err
		}
	}
	return r.buildReader(ctx, reg)
}

func (r *Runtime) Stop() error {
	r.initiateStop()
	r.workers.Wait()
	r.cleanupOnce.Do(func() {
		r.lifecycleMu.Lock()
		prepared := r.prepared
		r.prepared = nil
		r.lifecycleMu.Unlock()
		for _, reader := range prepared {
			_ = r.closeReader(reader)
		}
		r.cleanupMu.Lock()
		r.cleanupErr = r.cleanupHistory.Take()
		r.cleanupMu.Unlock()
		r.finish()
	})
	r.cleanupMu.Lock()
	defer r.cleanupMu.Unlock()
	return r.cleanupErr
}

func permanentFactoryError(err error) bool {
	return errors.Is(err, eventadapter.ErrInvalidAdapter) || errors.Is(err, eventadapter.ErrUnsupportedCapability) || errors.Is(err, eventadapter.ErrInvalidState)
}

func (r *Runtime) closeReader(reader Reader) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if cause, ok := recovered.(error); ok {
				err = fmt.Errorf("consumer reader cleanup panicked: %w", cause)
			} else {
				err = fmt.Errorf("consumer reader cleanup panicked: %v", recovered)
			}
		}
		if err != nil {
			r.cleanupMu.Lock()
			r.cleanupHistory.Add(err)
			r.cleanupMu.Unlock()
			log.Printf("[Event Consumer] Reader cleanup failed: %v", err)
		}
	}()
	return reader.Close()
}

func (r *Runtime) initiateStop() {
	r.stopOnce.Do(func() {
		r.lifecycleMu.Lock()
		r.stopped = true
		cancel := r.cancel
		r.lifecycleMu.Unlock()
		if cancel != nil {
			cancel() // 모든 goroutine 중지
		}
	})
}

func (r *Runtime) finish() {
	r.doneOnce.Do(func() {
		r.lifecycleMu.Lock()
		r.stopped = true
		r.lifecycleMu.Unlock()
		close(r.done)
		log.Printf("[Event Consumer] All consumers stopped")
	})
}
