package consumer

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/NARUBROWN/spine/internal/event/publish"
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
}

func NewRuntime(registry *Registry, factory runnerFactory, pipeline *pipeline.Pipeline) *Runtime {
	if registry == nil {
		panic("consumer: 레지스트리는 nil일 수 없습니다")
	}
	if factory == nil {
		panic("consumer: factory는 nil일 수 없습니다")
	}
	if pipeline == nil {
		panic("consumer: pipeline은 nil일 수 없습니다")
	}

	return &Runtime{
		registry: registry,
		factory:  factory,
		pipeline: pipeline,
		errChan:  make(chan error, max(1, len(registry.Registrations()))),
	}
}

// Errors는 런타임 내부에서 발생한 치명적 에러를 전달받기 위한 채널입니다.
// 채널은 close되지 않으므로, 필요 시 선택적으로 1개 이벤트를 대기하거나
// non-blocking 방식으로 조회하세요.
func (r *Runtime) Errors() <-chan error {
	return r.errChan
}

func (r *Runtime) Start(ctx context.Context) {
	ctx, r.cancel = context.WithCancel(ctx)
	for _, registration := range r.registry.Registrations() {
		log.Printf("[Event Consumer] 토픽 '%s'에 대한 컨슈머를 시작합니다.", registration.Topic)
		go func(reg Registration) {
			reader, err := r.factory.Build(reg)
			if err != nil {
				startErr := fmt.Errorf(
					"[Event Consumer] 컨슈머 초기화 실패 (topic=%s): %w",
					reg.Topic,
					err,
				)
				select {
				case r.errChan <- startErr:
				default:
					log.Printf("%v (에러 채널이 가득 차 전파하지 못했습니다)", startErr)
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
						log.Printf("[Event Consumer] 메시지 읽기 실패: %v", err)
						continue
					}

					eventBus := publish.NewEventBus()

					// Consumer RequestContext 생성 (Execution Context)
					reqCtx := NewRequestContext(ctx, msg, eventBus)

					// 핸들러 실행
					if err := r.pipeline.Execute(reqCtx); err != nil {
						log.Printf(
							"[Event Consumer] 핸들러 실행 실패 (%s): %v",
							reg.Topic,
							err,
						)
						// 핸들러 실패 시 NACK
						if nackErr := msg.Nack(); nackErr != nil {
							log.Printf(
								"[Event Consumer] NACK 실패 (%s): %v",
								reg.Topic,
								nackErr,
							)
						}
						continue
					}

					// 핸들러 성공 시 ACK
					if ackErr := msg.Ack(); ackErr != nil {
						log.Printf(
							"[Event Consumer] ACK 실패 (%s): %v",
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
			return fmt.Errorf("Consumer 초기화 실패 (%s): %w", reg.Topic, err)
		}
		if err := reader.Close(); err != nil {
			return fmt.Errorf("Consumer 종료 실패 (%s): %w", reg.Topic, err)
		}
	}
	return nil
}

func (r *Runtime) Stop() {
	r.stopOnce.Do(func() {
		if r.cancel != nil {
			r.cancel() // 모든 goroutine 중지
		}
		log.Printf("[Event Consumer] 모든 컨슈머를 중지했습니다.")
	})
}
