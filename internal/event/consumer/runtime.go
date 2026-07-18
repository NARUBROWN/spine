package consumer

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/NARUBROWN/spine/internal/pipeline"
)

type runnerFactory interface {
	Build(reg Registration) (Reader, error)
}

type Runtime struct {
	registry *Registry
	factory  runnerFactory
	pipeline *pipeline.Pipeline
	stopOnce sync.Once
	cancel   context.CancelFunc
	errChan  chan error
	done     chan struct{}
}

func NewRuntime(registry *Registry, factory runnerFactory, pipeline *pipeline.Pipeline) *Runtime {
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
		registry: registry,
		factory:  factory,
		pipeline: pipeline,
		errChan:  make(chan error, max(1, len(registry.Registrations()))),
		done:     make(chan struct{}),
	}
}

// Errors는 런타임 내부에서 발생한 치명적 에러를 전달받기 위한 채널입니다.
// 채널은 close되지 않으므로, 필요 시 선택적으로 1개 이벤트를 대기하거나
// non-blocking 방식으로 조회하세요.
func (r *Runtime) Errors() <-chan error {
	return r.errChan
}

func (r *Runtime) Done() <-chan struct{} {
	return r.done
}

func (r *Runtime) Start(ctx context.Context) {
	ctx, r.cancel = context.WithCancel(ctx)
	for _, registration := range r.registry.Registrations() {
		log.Printf("[Event Consumer] Starting consumer for topic '%s'", registration.Topic)
		go func(reg Registration) {
			reader, err := r.factory.Build(reg)
			if err != nil {
				startErr := fmt.Errorf(
					"[Event Consumer] Consumer initialization failed (topic=%s): %w",
					reg.Topic,
					err,
				)
				select {
				case r.errChan <- startErr:
				default:
					log.Printf("%v (could not forward because the error channel is full)", startErr)
				}
				// 초기화 실패는 치명적이므로 전체 런타임을 중단한다.
				r.Stop()
				return
			}
			defer reader.Close()

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
						log.Printf("[Event Consumer] Failed to read message: %v", err)
						continue
					}

					// Consumer ExecutionContext 생성
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
							log.Printf(
								"[Event Consumer] NACK failed (%s): %v",
								reg.Topic,
								nackErr,
							)
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
					}
				}
			}
		}(registration)
	}
}

func (r *Runtime) Validate() error {
	for _, reg := range r.registry.Registrations() {
		reader, err := r.factory.Build(reg)
		if err != nil {
			return fmt.Errorf("Consumer initialization failed (%s): %w", reg.Topic, err)
		}
		if err := reader.Close(); err != nil {
			return fmt.Errorf("Consumer shutdown failed (%s): %w", reg.Topic, err)
		}
	}
	return nil
}

func (r *Runtime) Stop() {
	r.stopOnce.Do(func() {
		if r.cancel != nil {
			r.cancel() // 모든 goroutine 중지
		}
		close(r.done)
		log.Printf("[Event Consumer] All consumers stopped")
	})
}
