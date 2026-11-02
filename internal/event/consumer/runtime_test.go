package consumer

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/container"
	eventresolver "github.com/NARUBROWN/spine/internal/event/consumer/resolver"
	"github.com/NARUBROWN/spine/internal/invoker"
	"github.com/NARUBROWN/spine/internal/pipeline"
)

type runtimeTestRouter struct {
	meta core.HandlerMeta
}

func (r *runtimeTestRouter) Route(ctx core.ExecutionContext) (core.HandlerMeta, error) {
	return r.meta, nil
}

type runtimeTestReader struct {
	msg  *Message
	sent bool
}

func (r *runtimeTestReader) Read(ctx context.Context) (*Message, error) {
	if !r.sent {
		r.sent = true
		return r.msg, nil
	}

	<-ctx.Done()
	return nil, ctx.Err()
}

func (r *runtimeTestReader) Close() error { return nil }

type runtimeTestFactory struct {
	reader Reader
}

type lifecycleRuntimeTestReader struct {
	started chan struct{}
	stopped chan struct{}
	once    sync.Once
}

func (r *lifecycleRuntimeTestReader) Read(ctx context.Context) (*Message, error) {
	r.once.Do(func() { close(r.started) })
	<-ctx.Done()
	close(r.stopped)
	return nil, ctx.Err()
}

func (*lifecycleRuntimeTestReader) Close() error { return nil }

type lifecycleRuntimeTestFactory struct {
	builds atomic.Int32
	reader Reader
}

func (f *lifecycleRuntimeTestFactory) Build(Registration) (Reader, error) {
	f.builds.Add(1)
	return f.reader, nil
}

type failingRuntimeTestReader struct{ reads atomic.Int32 }

func (r *failingRuntimeTestReader) Read(ctx context.Context) (*Message, error) {
	r.reads.Add(1)
	return nil, errors.New("broker channel closed")
}

func (r *failingRuntimeTestReader) Close() error { return nil }

type reconnectingRuntimeTestFactory struct {
	builds atomic.Int32
	first  Reader
	second Reader
	policy TransportRetryPolicy
}

type scriptedBuildRuntimeTestFactory struct {
	builds   atomic.Int32
	failures int32
	reader   Reader
	policy   TransportRetryPolicy
}

func (f *scriptedBuildRuntimeTestFactory) Build(Registration) (Reader, error) {
	if f.builds.Add(1) <= f.failures {
		return nil, errors.New("broker unavailable")
	}
	return f.reader, nil
}

func (f *scriptedBuildRuntimeTestFactory) ConsumerRetryPolicy() TransportRetryPolicy {
	return f.policy
}

type sequenceRuntimeTestFactory struct {
	builds  atomic.Int32
	readers []Reader
	policy  TransportRetryPolicy
}

type validatingRuntimeTestFactory struct {
	validations atomic.Int32
	failures    int32
	builds      atomic.Int32
	reader      Reader
	policy      TransportRetryPolicy
}

func (f *validatingRuntimeTestFactory) ValidateStartup(context.Context, Registration) error {
	if f.validations.Add(1) <= f.failures {
		return errors.New("broker handshake failed")
	}
	return nil
}

func (f *validatingRuntimeTestFactory) Build(Registration) (Reader, error) {
	f.builds.Add(1)
	return f.reader, nil
}

func (f *validatingRuntimeTestFactory) ConsumerRetryPolicy() TransportRetryPolicy {
	return f.policy
}

func (f *sequenceRuntimeTestFactory) Build(Registration) (Reader, error) {
	index := int(f.builds.Add(1)) - 1
	if index >= len(f.readers) {
		return nil, errors.New("unexpected extra reader build")
	}
	return f.readers[index], nil
}

func (f *sequenceRuntimeTestFactory) ConsumerRetryPolicy() TransportRetryPolicy {
	return f.policy
}

type sequenceRuntimeTestReader struct {
	messages []*Message
	reads    atomic.Int32
	closed   chan struct{}
	closeOne sync.Once
}

func (r *sequenceRuntimeTestReader) Read(ctx context.Context) (*Message, error) {
	index := int(r.reads.Add(1)) - 1
	if index < len(r.messages) {
		return r.messages[index], nil
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func (r *sequenceRuntimeTestReader) Close() error {
	r.closeOne.Do(func() { close(r.closed) })
	return nil
}

type nilRuntimeTestFactory struct{}

func (*nilRuntimeTestFactory) Build(Registration) (Reader, error) { return nil, nil }

type exhaustingRuntimeTestFactory struct {
	builds atomic.Int32
	reader Reader
}

func (f *exhaustingRuntimeTestFactory) Build(Registration) (Reader, error) {
	if f.builds.Add(1) == 1 {
		return f.reader, nil
	}
	return nil, errors.New("broker remains unavailable")
}

func (*exhaustingRuntimeTestFactory) ConsumerRetryPolicy() TransportRetryPolicy {
	return NewTransportRetryPolicy(time.Millisecond, time.Millisecond, 1, 0.01, 1)
}

func (f *reconnectingRuntimeTestFactory) Build(reg Registration) (Reader, error) {
	if f.builds.Add(1) == 1 {
		return f.first, nil
	}
	return f.second, nil
}

func (f *reconnectingRuntimeTestFactory) ConsumerRetryPolicy() TransportRetryPolicy {
	return f.policy
}

func (f *runtimeTestFactory) Build(reg Registration) (Reader, error) {
	return f.reader, nil
}

type runtimeTestPostHook struct {
	err error
}

func (h *runtimeTestPostHook) AfterExecution(ctx core.ExecutionContext) error {
	return h.err
}

type failOnceRuntimeTestPostHook struct{ calls atomic.Int32 }

func (h *failOnceRuntimeTestPostHook) AfterExecution(core.ExecutionContext) error {
	if h.calls.Add(1) == 1 {
		return errors.New("first delivery fails")
	}
	return nil
}

type blockingRuntimeTestController struct {
	started chan struct{}
	release chan struct{}
}

func (c *blockingRuntimeTestController) Handle([]byte) {
	close(c.started)
	<-c.release
}

type drainingRuntimeTestReader struct {
	msg             *Message
	sent            bool
	contextCanceled chan struct{}
	closeStarted    chan struct{}
	closeRelease    chan struct{}
	cancelOne       sync.Once
	closeOne        sync.Once
}

func (r *drainingRuntimeTestReader) Read(ctx context.Context) (*Message, error) {
	if !r.sent {
		r.sent = true
		go func() {
			<-ctx.Done()
			r.cancelOne.Do(func() { close(r.contextCanceled) })
		}()
		return r.msg, nil
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func (r *drainingRuntimeTestReader) Close() error {
	r.closeOne.Do(func() {
		close(r.closeStarted)
		<-r.closeRelease
	})
	return nil
}

type runtimeTestController struct{}

func (c *runtimeTestController) Handle(payload []byte) {}

func (c *runtimeTestController) Panic(payload []byte) {
	panic("boom")
}

func newRuntimePipeline(t *testing.T, methodName string, hookErr error) *pipeline.Pipeline {
	t.Helper()

	ctr := container.New()
	if err := ctr.RegisterConstructor(func() *runtimeTestController {
		return &runtimeTestController{}
	}); err != nil {
		t.Fatalf("생성자 등록 실패: %v", err)
	}

	controllerType := reflect.TypeOf(&runtimeTestController{})
	method, ok := controllerType.MethodByName(methodName)
	if !ok {
		t.Fatalf("메서드를 찾을 수 없습니다: %s", methodName)
	}

	p := pipeline.NewPipeline(
		&runtimeTestRouter{
			meta: core.HandlerMeta{
				ControllerType: controllerType,
				Method:         method,
			},
		},
		invoker.NewInvoker(ctr),
	)

	p.AddArgumentResolver(&eventresolver.PayloadResolver{})
	if hookErr != nil {
		p.AddPostExecutionHook(&runtimeTestPostHook{err: hookErr})
	}

	return p
}

func newBlockingRuntimePipeline(t *testing.T, controller *blockingRuntimeTestController) *pipeline.Pipeline {
	t.Helper()

	ctr := container.New()
	if err := ctr.RegisterConstructor(func() *blockingRuntimeTestController { return controller }); err != nil {
		t.Fatalf("register blocking controller: %v", err)
	}
	controllerType := reflect.TypeOf(controller)
	method, ok := controllerType.MethodByName("Handle")
	if !ok {
		t.Fatal("blocking handler method not found")
	}
	p := pipeline.NewPipeline(
		&runtimeTestRouter{meta: core.HandlerMeta{ControllerType: controllerType, Method: method}},
		invoker.NewInvoker(ctr),
	)
	p.AddArgumentResolver(&eventresolver.PayloadResolver{})
	return p
}

func waitSignal(t *testing.T, ch <-chan string) string {
	t.Helper()

	select {
	case got := <-ch:
		return got
	case <-time.After(2 * time.Second):
		t.Fatal("신호 대기 타임아웃")
		return ""
	}
}

func TestRuntime_NackOnPostHookFailure(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("topic", (*runtimeTestController).Handle); err != nil {
		t.Fatalf("등록 실패: %v", err)
	}

	signals := make(chan string, 2)
	msg := &Message{
		EventName: "topic",
		Payload:   []byte(`hello`),
	}
	msg.SetAckHandler(func() error {
		signals <- "ack"
		return nil
	})
	msg.SetNackHandler(func() error {
		signals <- "nack"
		return nil
	})

	runtime := NewRuntime(
		registry,
		&runtimeTestFactory{reader: &runtimeTestReader{msg: msg}},
		newRuntimePipeline(t, "Handle", context.DeadlineExceeded),
	)

	runtime.Start(context.Background())
	defer runtime.Stop()

	if got := waitSignal(t, signals); got != "nack" {
		t.Fatalf("post hook 실패 시 NACK 되어야 합니다. 실제=%s", got)
	}
}

func TestRuntime_NackOnRecoveredPanic(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("topic", (*runtimeTestController).Panic); err != nil {
		t.Fatalf("등록 실패: %v", err)
	}

	signals := make(chan string, 2)
	msg := &Message{
		EventName: "topic",
		Payload:   []byte(`hello`),
	}
	msg.SetAckHandler(func() error {
		signals <- "ack"
		return nil
	})
	msg.SetNackHandler(func() error {
		signals <- "nack"
		return nil
	})

	runtime := NewRuntime(
		registry,
		&runtimeTestFactory{reader: &runtimeTestReader{msg: msg}},
		newRuntimePipeline(t, "Panic", nil),
	)

	runtime.Start(context.Background())
	defer runtime.Stop()

	if got := waitSignal(t, signals); got != "nack" {
		t.Fatalf("panic 복구 시 NACK 되어야 합니다. 실제=%s", got)
	}
}

func TestRuntime_WaitsBeforeRebuildingAfterReaderError(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("topic", (*runtimeTestController).Handle); err != nil {
		t.Fatalf("등록 실패: %v", err)
	}
	reader := &failingRuntimeTestReader{}
	factory := &reconnectingRuntimeTestFactory{
		first:  reader,
		second: &runtimeTestReader{},
		policy: TransportRetryPolicy{InitialDelay: 100 * time.Millisecond, MaxDelay: time.Second, Multiplier: 2},
	}
	runtime := NewRuntime(registry, factory, newRuntimePipeline(t, "Handle", nil))
	waitEntered := make(chan time.Duration, 1)
	runtime.waitRetry = func(ctx context.Context, delay time.Duration) bool {
		waitEntered <- delay
		<-ctx.Done()
		return false
	}
	runtime.Start(context.Background())
	t.Cleanup(runtime.Stop)

	if delay := <-waitEntered; delay != 100*time.Millisecond {
		t.Fatalf("unexpected first retry delay: %s", delay)
	}
	if got := reader.reads.Load(); got != 1 {
		t.Fatalf("reader must not be read again before retry wait completes, reads=%d", got)
	}
	if got := factory.builds.Load(); got != 1 {
		t.Fatalf("reader must not be rebuilt before retry wait completes, builds=%d", got)
	}
}

func TestRuntime_RetriesInitialBuildWithConfiguredPolicy(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("topic", (*runtimeTestController).Handle); err != nil {
		t.Fatalf("register consumer: %v", err)
	}
	acked := make(chan struct{}, 1)
	msg := &Message{EventName: "topic", Payload: []byte("payload")}
	msg.SetAckHandler(func() error { acked <- struct{}{}; return nil })
	factory := &scriptedBuildRuntimeTestFactory{
		failures: 2,
		reader:   &runtimeTestReader{msg: msg},
		policy: TransportRetryPolicy{
			InitialDelay: 3 * time.Millisecond,
			MaxDelay:     6 * time.Millisecond,
			Multiplier:   2,
			MaxAttempts:  2,
		},
	}
	runtime := NewRuntime(registry, factory, newRuntimePipeline(t, "Handle", nil))
	delays := make(chan time.Duration, 2)
	runtime.waitRetry = func(ctx context.Context, delay time.Duration) bool {
		delays <- delay
		return ctx.Err() == nil
	}
	runtime.Start(context.Background())
	t.Cleanup(runtime.Stop)

	select {
	case <-acked:
	case <-time.After(2 * time.Second):
		t.Fatal("initial build retries did not reach a working reader")
	}
	if got := factory.builds.Load(); got != 3 {
		t.Fatalf("initial build plus two retries expected, builds=%d", got)
	}
	if first, second := <-delays, <-delays; first != 3*time.Millisecond || second != 6*time.Millisecond {
		t.Fatalf("unexpected deterministic backoff sequence: %s, %s", first, second)
	}
}

func TestRuntime_StopsAfterInitialBuildRetriesAreExhausted(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("topic", (*runtimeTestController).Handle); err != nil {
		t.Fatalf("register consumer: %v", err)
	}
	factory := &scriptedBuildRuntimeTestFactory{
		failures: 100,
		policy: TransportRetryPolicy{
			InitialDelay: time.Millisecond,
			MaxDelay:     time.Millisecond,
			Multiplier:   1,
			MaxAttempts:  2,
		},
	}
	runtime := NewRuntime(registry, factory, newRuntimePipeline(t, "Handle", nil))
	runtime.waitRetry = func(ctx context.Context, _ time.Duration) bool { return ctx.Err() == nil }
	runtime.Start(context.Background())
	t.Cleanup(runtime.Stop)

	select {
	case err := <-runtime.Errors():
		if !strings.Contains(err.Error(), "initialization attempts exhausted") {
			t.Fatalf("unexpected runtime error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for initial build exhaustion")
	}
	select {
	case <-runtime.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("runtime did not finish after initial build retries were exhausted")
	}
	if got := factory.builds.Load(); got != 3 {
		t.Fatalf("initial build plus MaxAttempts retries expected, builds=%d", got)
	}
}

func TestRuntime_KafkaStyleNackInvalidatesReaderBeforeHigherOffset(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("topic", (*runtimeTestController).Handle); err != nil {
		t.Fatalf("register consumer: %v", err)
	}
	replayed := make(chan struct{}, 1)
	higherAck := make(chan struct{}, 1)
	failed := &Message{EventName: "topic", Payload: []byte("offset-10")}
	failed.SetNackHandler(func() error { return ErrReaderInvalidated })
	higher := &Message{EventName: "topic", Payload: []byte("offset-11")}
	higher.SetAckHandler(func() error { higherAck <- struct{}{}; return nil })
	retry := &Message{EventName: "topic", Payload: []byte("offset-10")}
	retry.SetAckHandler(func() error { replayed <- struct{}{}; return nil })
	first := &sequenceRuntimeTestReader{messages: []*Message{failed, higher}, closed: make(chan struct{})}
	second := &sequenceRuntimeTestReader{messages: []*Message{retry}, closed: make(chan struct{})}
	factory := &sequenceRuntimeTestFactory{readers: []Reader{first, second}, policy: TransportRetryPolicy{InitialDelay: time.Millisecond, MaxDelay: time.Millisecond, Multiplier: 1}}
	p := newRuntimePipeline(t, "Handle", nil)
	p.AddPostExecutionHook(&failOnceRuntimeTestPostHook{})
	runtime := NewRuntime(registry, factory, p)
	runtime.Start(context.Background())
	t.Cleanup(runtime.Stop)

	select {
	case <-replayed:
	case <-time.After(2 * time.Second):
		t.Fatal("failed offset was not redelivered through a rebuilt reader")
	}
	select {
	case <-first.closed:
	default:
		t.Fatal("NACK invalidation must close the current reader")
	}
	if got := first.reads.Load(); got != 1 {
		t.Fatalf("higher offset must not be fetched after NACK, reads=%d", got)
	}
	select {
	case <-higherAck:
		t.Fatal("higher offset was ACKed after the preceding offset failed")
	default:
	}
}

func TestRuntime_KafkaStyleNackWaitsBeforeRebuildingReader(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("topic", (*runtimeTestController).Handle); err != nil {
		t.Fatalf("register consumer: %v", err)
	}

	failed := &Message{EventName: "topic", Payload: []byte("poison")}
	failed.SetNackHandler(func() error { return ErrReaderInvalidated })
	first := &sequenceRuntimeTestReader{messages: []*Message{failed}, closed: make(chan struct{})}
	second := &runtimeTestReader{msg: &Message{EventName: "topic", Payload: []byte("retry")}}
	factory := &sequenceRuntimeTestFactory{
		readers: []Reader{first, second},
		policy: TransportRetryPolicy{
			InitialDelay: 25 * time.Millisecond,
			MaxDelay:     25 * time.Millisecond,
			Multiplier:   1,
		},
	}
	p := newRuntimePipeline(t, "Handle", nil)
	p.AddPostExecutionHook(&runtimeTestPostHook{err: errors.New("permanent handler failure")})
	runtime := NewRuntime(registry, factory, p)
	waitStarted := make(chan time.Duration, 1)
	releaseWait := make(chan struct{})
	runtime.waitRetry = func(ctx context.Context, delay time.Duration) bool {
		select {
		case waitStarted <- delay:
		default:
		}
		select {
		case <-releaseWait:
			return true
		case <-ctx.Done():
			return false
		}
	}
	runtime.Start(context.Background())
	t.Cleanup(func() {
		close(releaseWait)
		runtime.Stop()
	})

	select {
	case delay := <-waitStarted:
		if delay != 25*time.Millisecond {
			t.Fatalf("NACK 재처리 backoff = %s, want 25ms", delay)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("NACK 이후 재처리 backoff가 시작되지 않았습니다")
	}

	if got := factory.builds.Load(); got != 1 {
		t.Fatalf("backoff 완료 전에 reader를 재생성하면 안 됩니다: builds=%d", got)
	}
	select {
	case <-first.closed:
	default:
		t.Fatal("backoff 전에 실패한 reader는 닫혀야 합니다")
	}
}

func TestRuntime_AckFailureInvalidatesReaderBeforeHigherOffset(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("topic", (*runtimeTestController).Handle); err != nil {
		t.Fatalf("register consumer: %v", err)
	}
	replayed := make(chan struct{}, 1)
	higherAck := make(chan struct{}, 1)
	failedCommit := &Message{EventName: "topic", Payload: []byte("offset-20")}
	failedCommit.SetAckHandler(func() error { return errors.New("commit failed") })
	higher := &Message{EventName: "topic", Payload: []byte("offset-21")}
	higher.SetAckHandler(func() error { higherAck <- struct{}{}; return nil })
	retry := &Message{EventName: "topic", Payload: []byte("offset-20")}
	retry.SetAckHandler(func() error { replayed <- struct{}{}; return nil })
	first := &sequenceRuntimeTestReader{messages: []*Message{failedCommit, higher}, closed: make(chan struct{})}
	second := &sequenceRuntimeTestReader{messages: []*Message{retry}, closed: make(chan struct{})}
	factory := &sequenceRuntimeTestFactory{readers: []Reader{first, second}, policy: TransportRetryPolicy{InitialDelay: time.Millisecond, MaxDelay: time.Millisecond, Multiplier: 1}}
	runtime := NewRuntime(registry, factory, newRuntimePipeline(t, "Handle", nil))
	runtime.Start(context.Background())
	t.Cleanup(runtime.Stop)

	select {
	case <-replayed:
	case <-time.After(2 * time.Second):
		t.Fatal("uncommitted offset was not redelivered through a rebuilt reader")
	}
	if got := first.reads.Load(); got != 1 {
		t.Fatalf("higher offset must not be fetched after ACK failure, reads=%d", got)
	}
	select {
	case <-higherAck:
		t.Fatal("higher offset was ACKed after the preceding commit failed")
	default:
	}
}

func TestRuntime_RebuildsReaderAfterTransportError(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("topic", (*runtimeTestController).Handle); err != nil {
		t.Fatalf("등록 실패: %v", err)
	}

	signals := make(chan string, 1)
	msg := &Message{EventName: "topic", Payload: []byte("hello")}
	msg.SetAckHandler(func() error { signals <- "ack"; return nil })
	factory := &reconnectingRuntimeTestFactory{
		first:  &failingRuntimeTestReader{},
		second: &runtimeTestReader{msg: msg},
		policy: NewTransportRetryPolicy(5*time.Millisecond, 10*time.Millisecond, 2, 0.01, 3),
	}
	runtime := NewRuntime(registry, factory, newRuntimePipeline(t, "Handle", nil))
	runtime.Start(context.Background())
	t.Cleanup(runtime.Stop)

	if got := waitSignal(t, signals); got != "ack" {
		t.Fatalf("rebuilt reader should resume message handling, got %q", got)
	}
	if builds := factory.builds.Load(); builds < 2 {
		t.Fatalf("reader should be rebuilt, builds=%d", builds)
	}
}

func TestRuntime_ValidateRejectsNilReader(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("topic", (*runtimeTestController).Handle); err != nil {
		t.Fatalf("등록 실패: %v", err)
	}
	runtime := NewRuntime(registry, &nilRuntimeTestFactory{}, newRuntimePipeline(t, "Handle", nil))
	err := runtime.Validate()
	if err == nil || !strings.Contains(err.Error(), "consumer factory returned a nil reader") {
		t.Fatalf("nil reader must return a validation error, got %v", err)
	}
}

func TestRuntime_ValidateWithRetryUsesConfiguredInitialConnectionPolicy(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("topic", (*runtimeTestController).Handle); err != nil {
		t.Fatalf("register consumer: %v", err)
	}
	factory := &scriptedBuildRuntimeTestFactory{
		failures: 2,
		reader:   &runtimeTestReader{},
		policy: TransportRetryPolicy{
			InitialDelay: 3 * time.Millisecond,
			MaxDelay:     6 * time.Millisecond,
			Multiplier:   2,
			MaxAttempts:  2,
		},
	}
	runtime := NewRuntime(registry, factory, newRuntimePipeline(t, "Handle", nil))
	var delays []time.Duration
	runtime.waitRetry = func(ctx context.Context, delay time.Duration) bool {
		delays = append(delays, delay)
		return ctx.Err() == nil
	}

	if err := runtime.ValidateWithRetry(context.Background()); err != nil {
		t.Fatalf("validation should recover after transient initial failures: %v", err)
	}
	if got := factory.builds.Load(); got != 3 {
		t.Fatalf("initial build plus two retries expected, builds=%d", got)
	}
	if len(delays) != 2 || delays[0] != 3*time.Millisecond || delays[1] != 6*time.Millisecond {
		t.Fatalf("unexpected validation backoff sequence: %v", delays)
	}
}

func TestRuntime_ValidateWithRetryRetriesFactoryStartupHandshakeBeforeBuild(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("topic", (*runtimeTestController).Handle); err != nil {
		t.Fatalf("register consumer: %v", err)
	}
	factory := &validatingRuntimeTestFactory{
		failures: 2,
		reader:   &runtimeTestReader{},
		policy: TransportRetryPolicy{
			InitialDelay: time.Millisecond,
			MaxDelay:     2 * time.Millisecond,
			Multiplier:   2,
			MaxAttempts:  2,
		},
	}
	runtime := NewRuntime(registry, factory, newRuntimePipeline(t, "Handle", nil))
	runtime.waitRetry = func(ctx context.Context, _ time.Duration) bool { return ctx.Err() == nil }

	if err := runtime.ValidateWithRetry(context.Background()); err != nil {
		t.Fatalf("startup handshake should recover with ConsumerRetry: %v", err)
	}
	if got := factory.validations.Load(); got != 3 {
		t.Fatalf("startup handshake attempts = %d, want 3", got)
	}
	if got := factory.builds.Load(); got != 1 {
		t.Fatalf("lazy reader must be built only after readiness succeeds: %d", got)
	}
}

func TestRuntime_ValidateWithRetryCanBeCanceledWhenRetriesAreUnlimited(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("topic", (*runtimeTestController).Handle); err != nil {
		t.Fatalf("register consumer: %v", err)
	}
	factory := &scriptedBuildRuntimeTestFactory{
		failures: 100,
		policy:   TransportRetryPolicy{InitialDelay: time.Millisecond, MaxDelay: time.Millisecond, Multiplier: 1},
	}
	runtime := NewRuntime(registry, factory, newRuntimePipeline(t, "Handle", nil))
	ctx, cancel := context.WithCancel(context.Background())
	runtime.waitRetry = func(context.Context, time.Duration) bool {
		cancel()
		return false
	}

	err := runtime.ValidateWithRetry(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled startup validation must preserve context cancellation: %v", err)
	}
	if got := factory.builds.Load(); got != 1 {
		t.Fatalf("cancellation must stop before another build, builds=%d", got)
	}
}

func TestRuntime_StartAfterStopDoesNotStartConsumers(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("topic", (*runtimeTestController).Handle); err != nil {
		t.Fatalf("register consumer: %v", err)
	}
	reader := &lifecycleRuntimeTestReader{started: make(chan struct{}), stopped: make(chan struct{})}
	factory := &lifecycleRuntimeTestFactory{reader: reader}
	runtime := NewRuntime(registry, factory, newRuntimePipeline(t, "Handle", nil))

	runtime.Stop()
	runtime.Start(context.Background())

	if got := factory.builds.Load(); got != 0 {
		t.Fatalf("Start after Stop must not build consumers: builds=%d", got)
	}
	select {
	case <-runtime.Done():
	default:
		t.Fatal("stopped runtime must keep Done closed")
	}
}

func TestRuntime_StopCancelsPublishedStartContext(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("topic", (*runtimeTestController).Handle); err != nil {
		t.Fatalf("register consumer: %v", err)
	}
	reader := &lifecycleRuntimeTestReader{started: make(chan struct{}), stopped: make(chan struct{})}
	runtime := NewRuntime(
		registry,
		&lifecycleRuntimeTestFactory{reader: reader},
		newRuntimePipeline(t, "Handle", nil),
	)

	runtime.Start(context.Background())
	select {
	case <-reader.started:
	case <-time.After(2 * time.Second):
		t.Fatal("consumer did not start")
	}
	runtime.Stop()
	select {
	case <-reader.stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not cancel the context published by Start")
	}
}

func TestRuntime_StopAndDoneWaitForHandlerAckAndReaderClose(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("topic", (*blockingRuntimeTestController).Handle); err != nil {
		t.Fatalf("register consumer: %v", err)
	}
	handlerStarted := make(chan struct{})
	handlerRelease := make(chan struct{})
	ackStarted := make(chan struct{})
	ackRelease := make(chan struct{})
	msg := &Message{EventName: "topic", Payload: []byte("payload")}
	msg.SetAckHandler(func() error {
		close(ackStarted)
		<-ackRelease
		return nil
	})
	reader := &drainingRuntimeTestReader{
		msg:             msg,
		contextCanceled: make(chan struct{}),
		closeStarted:    make(chan struct{}),
		closeRelease:    make(chan struct{}),
	}
	runtime := NewRuntime(
		registry,
		&runtimeTestFactory{reader: reader},
		newBlockingRuntimePipeline(t, &blockingRuntimeTestController{started: handlerStarted, release: handlerRelease}),
	)
	runtime.Start(context.Background())
	select {
	case <-handlerStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not start")
	}

	stopReturned := make(chan struct{})
	go func() {
		runtime.Stop()
		close(stopReturned)
	}()
	select {
	case <-reader.contextCanceled:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not cancel the worker context")
	}
	assertRuntimeStillDraining(t, runtime, stopReturned, "handler")

	close(handlerRelease)
	select {
	case <-ackStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("ACK did not start after the handler completed")
	}
	assertRuntimeStillDraining(t, runtime, stopReturned, "ACK")

	close(ackRelease)
	select {
	case <-reader.closeStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("reader Close did not start after ACK completed")
	}
	assertRuntimeStillDraining(t, runtime, stopReturned, "reader Close")

	close(reader.closeRelease)
	select {
	case <-stopReturned:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return after the worker fully drained")
	}
	select {
	case <-runtime.Done():
	default:
		t.Fatal("Done must close when Stop returns")
	}
}

func assertRuntimeStillDraining(t *testing.T, runtime *Runtime, stopReturned <-chan struct{}, stage string) {
	t.Helper()
	select {
	case <-runtime.Done():
		t.Fatalf("Done closed while %s was still running", stage)
	default:
	}
	select {
	case <-stopReturned:
		t.Fatalf("Stop returned while %s was still running", stage)
	default:
	}
}

func TestRuntime_ConcurrentStartStopIsRaceFreeAndStartsAtMostOnce(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("topic", (*runtimeTestController).Handle); err != nil {
		t.Fatalf("register consumer: %v", err)
	}
	p := newRuntimePipeline(t, "Handle", nil)

	for range 200 {
		reader := &lifecycleRuntimeTestReader{started: make(chan struct{}), stopped: make(chan struct{})}
		factory := &lifecycleRuntimeTestFactory{reader: reader}
		runtime := NewRuntime(registry, factory, p)
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			runtime.Start(context.Background())
		}()
		go func() {
			defer wg.Done()
			<-start
			runtime.Stop()
		}()
		close(start)
		wg.Wait()

		// 동시 상태 전환 이후 다시 호출해도 Start는 한 번만 실행됩니다.
		runtime.Start(context.Background())
		if got := factory.builds.Load(); got > 1 {
			t.Fatalf("consumer was built more than once: builds=%d", got)
		}
	}
}

func TestRuntime_StopsAfterTransportReconnectAttemptsAreExhausted(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("topic", (*runtimeTestController).Handle); err != nil {
		t.Fatalf("등록 실패: %v", err)
	}
	factory := &exhaustingRuntimeTestFactory{reader: &failingRuntimeTestReader{}}
	runtime := NewRuntime(registry, factory, newRuntimePipeline(t, "Handle", nil))
	runtime.waitRetry = func(ctx context.Context, _ time.Duration) bool { return ctx.Err() == nil }
	runtime.Start(context.Background())
	t.Cleanup(runtime.Stop)

	select {
	case err := <-runtime.Errors():
		if !strings.Contains(err.Error(), "reconnect attempts exhausted") {
			t.Fatalf("unexpected runtime error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for exhausted reconnect error")
	}
	select {
	case <-runtime.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("runtime did not stop after reconnect attempts were exhausted")
	}
}
