package publish

import (
	"context"
	"errors"
	"fmt"
	"sync"

	eventadapter "github.com/NARUBROWN/spine/adapter/event"
)

// PlatformPublisher는 실패한 발행기를 분리하고 다음 호출에서 새 자원을 준비합니다.
// 실패한 메시지를 자동 재발행하지 않으며 정상 발행의 병렬성은 유지합니다.
type PlatformPublisher struct {
	mu           sync.Mutex
	factory      eventadapter.Factory
	current      *publisherInstance
	instances    []*publisherInstance
	creating     chan struct{}
	cancelCreate context.CancelFunc
	closing      bool
	closeOnce    sync.Once
	closeDone    chan struct{}
	closeErr     error
}

type publisherInstance struct {
	owner     *PlatformPublisher
	native    eventadapter.Publisher
	active    int
	retired   bool
	closeOnce sync.Once
	done      chan struct{}
}

func NewPlatformPublisher(factory eventadapter.Factory, native eventadapter.Publisher) *PlatformPublisher {
	p := &PlatformPublisher{factory: factory, closeDone: make(chan struct{})}
	instance := &publisherInstance{owner: p, native: native, done: make(chan struct{})}
	p.current = instance
	p.instances = []*publisherInstance{instance}
	return p
}

func (p *PlatformPublisher) acquire(ctx context.Context) (*publisherInstance, error) {
	for {
		p.mu.Lock()
		if p.closing {
			p.mu.Unlock()
			return nil, eventadapter.ErrInvalidState
		}
		if p.current != nil {
			instance := p.current
			instance.active++
			p.mu.Unlock()
			return instance, nil
		}
		if p.creating != nil {
			ready := p.creating
			p.mu.Unlock()
			select {
			case <-ready:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		ready := make(chan struct{})
		p.creating = ready
		createCtx, cancel := context.WithCancel(ctx)
		p.cancelCreate = cancel
		p.mu.Unlock()
		native, err := createPlatformPublisher(createCtx, p.factory)
		cancel()
		if err == nil && ctx.Err() != nil {
			err = ctx.Err()
		}
		p.mu.Lock()
		p.creating = nil
		p.cancelCreate = nil
		var instance *publisherInstance
		if !isNilEventPublisher(native) {
			instance = &publisherInstance{owner: p, native: native, done: make(chan struct{})}
			p.instances = append(p.instances, instance)
		}
		if err == nil && instance == nil {
			err = fmt.Errorf("%w: factory returned a nil publisher", eventadapter.ErrInvalidAdapter)
		}
		if p.closing && err == nil {
			err = eventadapter.ErrInvalidState
		}
		if instance != nil {
			if err != nil {
				instance.retired = true
				instance.startClose()
			} else {
				instance.active = 1
				p.current = instance
			}
		}
		close(ready)
		p.mu.Unlock()
		return instance, err
	}
}

func createPlatformPublisher(ctx context.Context, factory eventadapter.Factory) (native eventadapter.Publisher, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if cause, ok := recovered.(error); ok {
				err = fmt.Errorf("publisher factory panicked: %w", cause)
			} else {
				err = fmt.Errorf("publisher factory panicked: %v", recovered)
			}
		}
	}()
	return factory.NewPublisher(ctx)
}

func (p *PlatformPublisher) Publish(ctx context.Context, message eventadapter.Message) (resultErr error) {
	if ctx == nil {
		return fmt.Errorf("%w: context is nil", eventadapter.ErrInvalidAdapter)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	instance, err := p.acquire(ctx)
	if err != nil {
		return err
	}
	completed := false
	defer func() {
		p.mu.Lock()
		instance.active--
		// 다른 발행이 진행 중이면 모두 끝난 뒤 실패한 자원을 정리합니다.
		if !completed || resultErr != nil || p.closing {
			instance.retired = true
			if p.current == instance {
				p.current = nil
			}
		}
		if instance.retired && instance.active == 0 {
			instance.startClose()
		}
		p.mu.Unlock()
	}()
	resultErr = instance.native.Publish(ctx, message)
	completed = true
	return resultErr
}

func (i *publisherInstance) startClose() {
	i.closeOnce.Do(func() {
		go func() {
			defer close(i.done)
			err := func() (closeErr error) {
				defer func() {
					if recovered := recover(); recovered != nil {
						if cause, ok := recovered.(error); ok {
							closeErr = fmt.Errorf("publisher cleanup panicked: %w", cause)
						} else {
							closeErr = fmt.Errorf("publisher cleanup panicked: %v", recovered)
						}
					}
				}()
				return i.native.Close(context.Background())
			}()
			p := i.owner
			p.mu.Lock()
			if err != nil {
				p.closeErr = errors.Join(p.closeErr, err)
			}
			// 닫은 native 인스턴스를 장시간 실행하는 App에 계속 보관하지 않습니다.
			for index, instance := range p.instances {
				if instance == i {
					p.instances = append(p.instances[:index], p.instances[index+1:]...)
					break
				}
			}
			p.mu.Unlock()
		}()
	})
}

func (p *PlatformPublisher) Close(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("%w: context is nil", eventadapter.ErrInvalidAdapter)
	}
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.closing = true
		if p.cancelCreate != nil {
			p.cancelCreate()
		}
		creating := p.creating
		p.mu.Unlock()
		go func() {
			defer close(p.closeDone)
			if creating != nil {
				<-creating
			}
			p.mu.Lock()
			instances := append([]*publisherInstance(nil), p.instances...)
			for _, instance := range instances {
				instance.retired = true
				if instance.active == 0 {
					instance.startClose()
				}
			}
			p.current = nil
			p.mu.Unlock()
			for _, instance := range instances {
				<-instance.done
			}
		}()
	})
	select {
	case <-p.closeDone:
		return p.closeErr
	default:
	}
	select {
	case <-p.closeDone:
		return p.closeErr
	case <-ctx.Done():
		return ctx.Err()
	}
}
