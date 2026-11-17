package test

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NARUBROWN/spine"
	echo "github.com/NARUBROWN/spine-echo-platform"
	gorilla "github.com/NARUBROWN/spine-gorilla-platform"
	event "github.com/NARUBROWN/spine/contract/event"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/api"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/event/publish"
	"github.com/NARUBROWN/spine/pkg/httpx"
	handlerspine "github.com/NARUBROWN/spine/pkg/spine"
	pkgws "github.com/NARUBROWN/spine/pkg/ws"
	"github.com/gorilla/websocket"
)

type publisherCapture struct{ publisher publish.Publisher }

func (c *publisherCapture) Init(container core.Container) error {
	p, err := container.Resolve(reflect.TypeFor[publish.Publisher]())
	if err == nil {
		c.publisher = p.(publish.Publisher)
	}
	return err
}
func (*publisherCapture) Start() error               { return nil }
func (*publisherCapture) Stop(context.Context) error { return nil }

type appPublisherFactory struct{ native *appNativePublisher }

func (appPublisherFactory) Capabilities() event.Capabilities {
	return event.Capabilities{ContractVersion: event.ContractVersion, Publish: true}
}
func (f appPublisherFactory) NewPublisher(context.Context) (event.Publisher, error) {
	return f.native, nil
}
func (appPublisherFactory) NewReader(context.Context, event.Subscription) (event.Reader, error) {
	return nil, event.ErrUnsupportedCapability
}

type appNativePublisher struct {
	sends, closes atomic.Int32
	send          func(context.Context, event.Message) error
	close         func() error
}

func (p *appNativePublisher) Publish(ctx context.Context, m event.Message) error {
	p.sends.Add(1)
	if p.send != nil {
		return p.send(ctx, m)
	}
	return nil
}
func (p *appNativePublisher) Close(context.Context) error {
	p.closes.Add(1)
	if p.close != nil {
		return p.close()
	}
	return nil
}

type publisherController struct{ publisher publish.Publisher }

func (c *publisherController) RegisterRoutes(r *api.Registry) {
	r.POST("/publish/:result", func(ctx handlerspine.Ctx) (httpx.Response[any], error) {
		request, _ := ctx.HTTPRequestContext()
		if err := c.publisher.Event(ctx.ExecutionContext().Context(), "orders", publish.Fields{"source": "http"}); err != nil {
			return httpx.Response[any]{}, err
		}
		switch request.Param("result") {
		case "handler-error":
			return httpx.Response[any]{}, errors.New("later handler error")
		case "response-error":
			return httpx.Response[any]{Body: make(chan int)}, nil
		}
		return httpx.Response[any]{Body: "sent"}, nil
	})
}
func (c *publisherController) Message(ctx handlerspine.Ctx, _ []byte) error {
	if err := c.publisher.Event(ctx.ExecutionContext().Context(), "orders", publish.Fields{"source": "ws"}); err != nil {
		return err
	}
	return pkgws.Send(ctx.ExecutionContext().Context(), pkgws.TextMessage, []byte("sent"))
}
func TestAppPublisherSharesDIInstanceAcrossHTTPWebSocketAndPolling(t *testing.T) {
	native := &appNativePublisher{}
	capture := &publisherCapture{}
	app := spine.New(echo.Option(), gorilla.Option(), spine.WithEventAdapter("events", appPublisherFactory{native}))
	app.RegisterTransport(capture)
	app.Constructor(func(p publish.Publisher) *publisherController {
		if p != capture.publisher {
			t.Error("DI created a second application publisher")
		}
		return &publisherController{p}
	})
	if err := app.WebSocket().Register("/ws/publish", (*publisherController).Message); err != nil {
		t.Fatal(err)
	}
	if err := app.Validate(boot.Options{HTTP: &boot.HTTPOptions{}}); err != nil {
		t.Fatal(err)
	}
	prepared, err := app.Prepare(t.Context(), boot.Options{HTTP: &boot.HTTPOptions{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := prepared.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	if err := capture.publisher.Event(t.Context(), "orders", nil); err != nil {
		t.Fatal(err)
	} // No request has occurred.
	handler, err := spine.NetHTTPHandler(prepared)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	for _, result := range []string{"handler-error", "response-error"} {
		before := native.sends.Load()
		response, err := server.Client().Post(server.URL+"/publish/"+result, "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 500 || native.sends.Load() != before+1 {
			t.Fatalf("%s: status=%d sends=%d", result, response.StatusCode, native.sends.Load())
		}
	}
	const count = 16
	var wg sync.WaitGroup
	for range count {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := capture.publisher.Event(t.Context(), "orders", nil); err != nil {
				t.Error(err)
			}
		}()
		go func() {
			defer wg.Done()
			response, err := server.Client().Post(server.URL+"/publish/ok", "application/json", nil)
			if err != nil {
				t.Error(err)
				return
			}
			response.Body.Close()
			if response.StatusCode != 200 {
				t.Errorf("HTTP status=%d", response.StatusCode)
			}
		}()
	}
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws/publish", nil)
	if err != nil {
		t.Fatal(err)
	}
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for range count {
		if err := conn.WriteMessage(websocket.TextMessage, []byte(`{}`)); err != nil {
			t.Fatal(err)
		}
		if _, body, err := conn.ReadMessage(); err != nil || string(body) != "sent" {
			t.Fatalf("%s %v", body, err)
		}
	}
	conn.Close()
	wg.Wait()
	if got := native.sends.Load(); got != 3+3*count {
		t.Fatalf("sends=%d", got)
	}
}

func TestAppPublisherPublishOnlyShutdownHookStopsWorkerBeforeNativeClose(t *testing.T) {
	started, workerDone := make(chan struct{}), make(chan struct{})
	native := &appNativePublisher{send: func(ctx context.Context, _ event.Message) error { close(started); <-ctx.Done(); return ctx.Err() }, close: func() error {
		select {
		case <-workerDone:
			return nil
		default:
			return errors.New("worker still running")
		}
	}}
	capture := &publisherCapture{}
	workerCtx, stopWorker := context.WithCancel(t.Context())
	defer stopWorker()
	app := spine.New(spine.WithEventAdapter("events", appPublisherFactory{native}), spine.WithShutdownHook(func(ctx context.Context, p publish.Publisher) error {
		if p != capture.publisher {
			return errors.New("shutdown resolved another publisher")
		}
		stopWorker()
		select {
		case <-workerDone:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}))
	app.RegisterTransport(capture)
	prepared, err := app.Prepare(t.Context(), boot.Options{})
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		defer close(workerDone)
		if err := capture.publisher.Event(workerCtx, "orders", nil); !errors.Is(err, context.Canceled) {
			t.Errorf("worker error=%v", err)
		}
	}()
	<-started
	if err := prepared.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := prepared.Close(t.Context()); err != nil || native.closes.Load() != 1 {
		t.Fatalf("close=%v count=%d", err, native.closes.Load())
	}
	if err := capture.publisher.Event(t.Context(), "orders", nil); !errors.Is(err, event.ErrInvalidState) {
		t.Fatalf("closed call=%v", err)
	}
}
func TestAppPublisherMissingPlatformReturnsExplicitError(t *testing.T) {
	capture := &publisherCapture{}
	app := spine.New()
	app.RegisterTransport(capture)
	prepared, err := app.Prepare(t.Context(), boot.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close(context.Background())
	if err := capture.publisher.Event(t.Context(), "orders", nil); !errors.Is(err, publish.ErrUnavailable) {
		t.Fatal(fmt.Sprint(err))
	}
}
