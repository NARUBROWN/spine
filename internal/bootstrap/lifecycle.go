package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/NARUBROWN/spine/contract/description"
	"github.com/NARUBROWN/spine/internal/endpoint"
	"log"
	"sync"
	"time"

	adapter "github.com/NARUBROWN/spine/contract/http"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/event/consumer"
	"github.com/NARUBROWN/spine/internal/ws"
)

var ErrInvalidContext = errors.New("context must not be nil")

type consumerRuntime struct {
	name    string
	runtime *consumer.Runtime
}

type publisherCloser struct {
	name  string
	close func(context.Context) error
}

// Application은 준비가 성공한 자원만 보관하고 공통 정리 순서를 관리합니다.
type Application struct {
	config          Config
	description     description.HTTP
	descriptionOnce sync.Once
	descriptionErr  error
	server          adapter.HTTPAdapter
	listener        *managedHTTPListener
	wsRuntime       *ws.Runtime
	consumers       []consumerRuntime
	transports      []core.CustomTransport
	publishers      []publisherCloser
	closeOnce       sync.Once
	closeDone       chan struct{}
	closeErr        error
}

func (a *Application) HTTPAdapter() adapter.HTTPAdapter { return a.server }

func (a *Application) shutdownTimeout() time.Duration {
	if a.config.ShutdownTimeout > 0 {
		return a.config.ShutdownTimeout
	}
	return 10 * time.Second
}

func (a *Application) closeWithTimeout() error {
	ctx, cancel := context.WithTimeout(context.Background(), a.shutdownTimeout())
	defer cancel()
	return a.Close(ctx)
}

// Close는 정리를 한 번 시작하고 각 호출의 context로 완료를 기다립니다.
// 첫 호출의 취소를 정리 작업에 전파하지 않아 진행 중인 요청의 자원을 보존합니다.
func (a *Application) Close(ctx context.Context) error {
	if isNilHTTPValue(ctx) {
		return ErrInvalidContext
	}
	a.closeOnce.Do(func() {
		if a.server != nil {
			a.server.StopAccepting()
		}
		go a.closeResources()
	})
	select {
	case <-a.closeDone:
		return a.closeErr
	default:
	}
	select {
	case <-a.closeDone:
		return a.closeErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *Application) closeResources() {
	defer close(a.closeDone)
	step := func(name string, close func() error) {
		if err := closeResource(name, close); err != nil {
			a.closeErr = errors.Join(a.closeErr, err)
			log.Printf("[Bootstrap] %v", err)
		}
	}
	if a.wsRuntime != nil {
		step("WebSocket runtime", func() error { a.wsRuntime.Stop(); return nil })
	}
	if a.server != nil {
		step("HTTP adapter", func() error { return a.server.Shutdown(context.Background()) })
	}
	if a.listener != nil {
		step("HTTP listener", a.listener.Close)
	}
	for i := len(a.consumers) - 1; i >= 0; i-- {
		runtime := a.consumers[i]
		step(runtime.name+" consumer", runtime.runtime.Stop)
	}
	ctx, cancel := context.WithTimeout(context.Background(), a.shutdownTimeout())
	defer cancel()
	for i := len(a.transports) - 1; i >= 0; i-- {
		transport := a.transports[i]
		step("custom transport", func() error { return transport.Stop(ctx) })
	}
	for i := len(a.publishers) - 1; i >= 0; i-- {
		publisher := a.publishers[i]
		step(publisher.name+" publisher", func() error { return publisher.close(context.Background()) })
	}
}

// 정리 콜백 하나의 패닉이 다른 자원 회수를 생략하거나 소유 고루틴을 중단하지 않게 합니다.
func closeResource(name string, close func() error) (err error) {
	defer func() {
		if value := recover(); value != nil {
			err = fmt.Errorf("%s cleanup panicked: %v", name, value)
		}
	}()
	if err = close(); err != nil {
		return fmt.Errorf("%s cleanup failed: %w", name, err)
	}
	return nil
}

// DescribeHTTP는 준비 시 고정한 등록에서 명세를 한 번 만들고 독립 복사본을 반환합니다.
func (a *Application) DescribeHTTP() (description.HTTP, error) {
	a.descriptionOnce.Do(func() {
		a.description, a.descriptionErr = endpoint.Describe(a.config.Routes, a.config.HTTP, a.config.Validator)
	})
	if a.descriptionErr != nil {
		return description.HTTP{}, a.descriptionErr
	}
	data, err := json.Marshal(a.description)
	if err != nil {
		return description.HTTP{}, err
	}
	var result description.HTTP
	err = json.Unmarshal(data, &result)
	return result, err
}
