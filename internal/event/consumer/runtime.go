package consumer

import (
	"context"
	"log"
	"sync"

	"github.com/NARUBROWN/spine/internal/event/publish"
)

type runnerFactory interface {
	Build(reg Registration) Reader
}

type Runtime struct {
	registry *Registry
	factory  runnerFactory
	invoker  *Invoker
	eventBus publish.EventBus
	stopOnce sync.Once
}

func NewRuntime(registry *Registry, factory runnerFactory, invoker *Invoker, eventBus publish.EventBus) *Runtime {
	if registry == nil {
		panic("consumer: 레지스트리는 nil일 수 없습니다")
	}
	if factory == nil {
		panic("consumer: factory는 nil일 수 없습니다")
	}
	if invoker == nil {
		panic("consumer: invoker는 nil일 수 없습니다")
	}
	if eventBus == nil {
		panic("consumer: eventBus는 nil일 수 없습니다")
	}

	return &Runtime{
		registry: registry,
		factory:  factory,
		invoker:  invoker,
		eventBus: eventBus,
	}
}

func (r *Runtime) Start(ctx context.Context) {
	for _, registration := range r.registry.Registrations() {
		log.Printf("[Event Consumer] 토픽 '%s'에 대한 컨슈머를 시작합니다.", registration.Topic)
		go func(reg Registration) {
			reader := r.factory.Build(reg)
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

					// Consumer RequestContext 생성 (Execution Context)
					reqCtx := NewRequestContext(
						ctx,
						msg,
						r.eventBus,
					)

					meta := reg.Meta

					// ArgumentResolver로 실행 인자 구성
					args, err := r.invoker.ResolveArguments(
						reqCtx,
						meta.Method,
					)
					if err != nil {
						log.Printf("[Event Consumer] 인자 해석 실패 (%s): %v", reg.Topic, err)
						continue
					}

					// Invoker 실행
					if _, err := r.invoker.Invoke(
						meta.ControllerType,
						meta.Method,
						args,
					); err != nil {
						log.Printf("[Event Consumer] 핸들러 실행 실패 (%s): %v", reg.Topic, err)
					}
				}
			}
		}(registration)
	}
}

func (r *Runtime) Stop() {
	r.stopOnce.Do(func() {
		log.Printf("[Event Consumer] 모든 컨슈머를 중지했습니다.")
	})
}
