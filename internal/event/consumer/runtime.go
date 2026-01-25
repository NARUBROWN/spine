package consumer

import (
	"context"
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
	}
}

func (r *Runtime) Start(ctx context.Context) {
	ctx, r.cancel = context.WithCancel(ctx)
	for _, registration := range r.registry.Registrations() {
		log.Printf("[Event Consumer] 토픽 '%s'에 대한 컨슈머를 시작합니다.", registration.Topic)
		go func(reg Registration) {
			reader, err := r.factory.Build(reg)
			if err != nil {
				log.Panicf(
					"[Event Consumer] 컨슈머 초기화 실패 (topic=%s): %v",
					reg.Topic,
					err,
				)
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

func (r *Runtime) Stop() {
	r.stopOnce.Do(func() {
		if r.cancel != nil {
			r.cancel() // 모든 goroutine 중지
		}
		log.Printf("[Event Consumer] 모든 컨슈머를 중지했습니다.")
	})
}
