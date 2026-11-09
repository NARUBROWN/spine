package bootstrap

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	adapter "github.com/NARUBROWN/spine/contract/http"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/boot"
)

type preparationAdapter struct {
	handler       http.Handler
	shutdown      func(context.Context) error
	serveCalls    atomic.Int32
	shutdownCalls atomic.Int32
	gateOnce      sync.Once
	gate          *preparedHandler
}

func (a *preparationAdapter) Handler() http.Handler {
	a.gateOnce.Do(func() {
		if a.handler != nil {
			a.gate = newPreparedHandler(a.handler)
		}
	})
	if a.gate == nil {
		return nil
	}
	return a.gate
}
func (a *preparationAdapter) StopAccepting() {
	a.Handler()
	if a.gate != nil {
		a.gate.stop()
	}
}
func (a *preparationAdapter) Serve(net.Listener, ...adapter.ServeOption) error {
	a.serveCalls.Add(1)
	return errors.New("unexpected Serve")
}
func (a *preparationAdapter) Shutdown(ctx context.Context) error {
	a.shutdownCalls.Add(1)
	a.StopAccepting()
	if a.gate != nil {
		<-a.gate.drained
	}
	if a.shutdown != nil {
		return a.shutdown(ctx)
	}
	return nil
}

type callbackTransport struct {
	init  func(core.Container) error
	start func() error
	stop  func(context.Context) error
}

func (t callbackTransport) Init(c core.Container) error {
	if t.init != nil {
		return t.init(c)
	}
	return nil
}
func (t callbackTransport) Start() error {
	if t.start != nil {
		return t.start()
	}
	return nil
}
func (t callbackTransport) Stop(ctx context.Context) error {
	if t.stop != nil {
		return t.stop(ctx)
	}
	return nil
}

func preparationFactory(a adapter.HTTPAdapter) *configurableFactory {
	return &configurableFactory{
		capabilities: adapter.Capabilities{ContractVersion: adapter.ContractVersion, NetHTTPHandler: true},
		newAdapter:   func(adapter.Config) (adapter.HTTPAdapter, error) { return a, nil },
	}
}

func TestPreparePreflightRunsBeforeCustomInitialization(t *testing.T) {
	transport := &testTransport{}
	factory := preparationFactory(nil)
	factory.capabilities.ContractVersion = "unsupported"
	factory.newAdapter = func(adapter.Config) (adapter.HTTPAdapter, error) {
		t.Fatal("기능 검증 실패 뒤 factory를 호출하면 안 됩니다")
		return nil, nil
	}
	prepared, err := Prepare(context.Background(), Config{HTTP: &boot.HTTPOptions{}, HTTPAdapterFactory: factory, CustomTransports: []core.CustomTransport{transport}})
	if prepared != nil || !errors.Is(err, adapter.ErrInvalidAdapter) || transport.initCalls.Load() != 0 {
		t.Fatalf("준비 사전 검증은 초기화 전에 실패해야 합니다: %v", err)
	}
}

func TestPrepareDoesNotListenOrStartRuntime(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	server := &preparationAdapter{handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })}
	transport := &testTransport{}
	prepared, err := Prepare(context.Background(), Config{
		Address: listener.Addr().String(), HTTP: &boot.HTTPOptions{}, HTTPAdapterFactory: preparationFactory(server),
		CustomTransports: []core.CustomTransport{transport},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := prepared.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	if server.serveCalls.Load() != 0 || transport.startCalls.Load() != 0 || transport.initCalls.Load() != 1 {
		t.Fatal("준비는 Init만 수행하고 Serve와 Start를 실행하면 안 됩니다")
	}
	handler := preparationHandler(t, prepared)
	if handler == nil || handler != preparationHandler(t, prepared) {
		t.Fatal("같은 준비 진입점을 반환해야 합니다")
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest("GET", "/", nil))
	if recorder.Code != 204 {
		t.Fatalf("응답 = %d", recorder.Code)
	}
	if err := prepared.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest("GET", "/", nil))
	if recorder.Code != 503 || server.shutdownCalls.Load() != 1 || transport.stopCalls.Load() != 1 {
		t.Fatal("Close 뒤 새 요청을 거부하고 자원을 한 번만 정리해야 합니다")
	}
}

func TestPrepareCanceledBeforeAndDuringInitRollsBack(t *testing.T) {
	for _, cancelInInit := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		var inits, starts, stops atomic.Int32
		transport := callbackTransport{
			init:  func(core.Container) error { inits.Add(1); cancel(); return nil },
			start: func() error { starts.Add(1); return nil },
			stop:  func(context.Context) error { stops.Add(1); return nil },
		}
		if !cancelInInit {
			cancel()
		}
		prepared, err := Prepare(ctx, Config{CustomTransports: []core.CustomTransport{transport}})
		if prepared != nil || !errors.Is(err, context.Canceled) || starts.Load() != 0 {
			t.Fatalf("준비 취소를 보존해야 합니다: %v", err)
		}
		want := int32(0)
		if cancelInInit {
			want = 1
		}
		if inits.Load() != want || stops.Load() != want {
			t.Fatalf("완료된 Init만 정리해야 합니다: %d/%d", inits.Load(), stops.Load())
		}
	}
}

func TestPrepareFactoryFailurePreservesCauseAndCleansPartialResult(t *testing.T) {
	want := errors.New("factory failed")
	cleanup := errors.New("shutdown failed")
	server := &preparationAdapter{shutdown: func(context.Context) error { return cleanup }}
	factory := preparationFactory(server)
	factory.newAdapter = func(adapter.Config) (adapter.HTTPAdapter, error) { return server, want }
	var order []string
	first := callbackTransport{stop: func(context.Context) error { order = append(order, "first"); return nil }}
	second := callbackTransport{stop: func(context.Context) error { order = append(order, "second"); return nil }}
	prepared, err := Prepare(context.Background(), Config{HTTP: &boot.HTTPOptions{}, HTTPAdapterFactory: factory, CustomTransports: []core.CustomTransport{first, second}})
	if prepared != nil || !errors.Is(err, want) || !errors.Is(err, cleanup) || strings.Join(order, ",") != "second,first" || server.shutdownCalls.Load() != 1 {
		t.Fatalf("준비 실패 원인과 역순 자원 회수를 보존해야 합니다: %v %v", err, order)
	}
}

func TestPreparePanicRollsBackAndPropagates(t *testing.T) {
	transport := &testTransport{}
	factory := preparationFactory(nil)
	factory.newAdapter = func(adapter.Config) (adapter.HTTPAdapter, error) { panic("native hook panic") }
	defer func() {
		if value := recover(); value != "native hook panic" || transport.stopCalls.Load() != 1 {
			t.Fatalf("패닉을 보존하고 초기화 자원은 정리해야 합니다: %v", value)
		}
	}()
	_, _ = Prepare(context.Background(), Config{HTTP: &boot.HTTPOptions{}, HTTPAdapterFactory: factory, CustomTransports: []core.CustomTransport{transport}})
	t.Fatal("준비 패닉을 삼키면 안 됩니다")
}

func TestPreparedCloseDeadlineAndConcurrentWaitsPreserveActiveRequest(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	requestDone := make(chan struct{})
	var releaseOnce sync.Once
	var stops, publishers atomic.Int32
	server := &preparationAdapter{handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		if stops.Load() != 0 || publishers.Load() != 0 {
			t.Error("진행 요청보다 먼저 자원을 닫으면 안 됩니다")
		}
		w.WriteHeader(202)
	})}
	transport := callbackTransport{stop: func(context.Context) error { stops.Add(1); return nil }}
	prepared, err := Prepare(context.Background(), Config{HTTP: &boot.HTTPOptions{}, HTTPAdapterFactory: preparationFactory(server), CustomTransports: []core.CustomTransport{transport}})
	if err != nil {
		t.Fatal(err)
	}
	prepared.publishers = append(prepared.publishers, publisherCloser{name: "test", close: func(context.Context) error { publishers.Add(1); return nil }})
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = prepared.Close(ctx)
	})
	if err := prepared.Close(nil); !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("nil Close context를 거부해야 합니다: %v", err)
	}
	recorder := httptest.NewRecorder()
	go func() {
		defer close(requestDone)
		preparationHandler(t, prepared).ServeHTTP(recorder, httptest.NewRequest("GET", "/", nil))
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("요청 시작 시간 초과")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := prepared.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close 대기 시간 초과를 반환해야 합니다: %v", err)
	}
	if stops.Load() != 0 || publishers.Load() != 0 {
		t.Fatal("시간 초과 후에도 진행 요청의 자원을 유지해야 합니다")
	}
	rejected := httptest.NewRecorder()
	preparationHandler(t, prepared).ServeHTTP(rejected, httptest.NewRequest("GET", "/", nil))
	if rejected.Code != 503 {
		t.Fatalf("종료 중 새 요청 = %d", rejected.Code)
	}
	releaseOnce.Do(func() { close(release) })
	var waits sync.WaitGroup
	for range 16 {
		waits.Add(1)
		go func() {
			defer waits.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := prepared.Close(ctx); err != nil {
				t.Error(err)
			}
		}()
	}
	waits.Wait()
	<-requestDone
	if recorder.Code != 202 || stops.Load() != 1 || publishers.Load() != 1 || server.shutdownCalls.Load() != 1 {
		t.Fatal("재대기와 동시 Close는 한 번의 정리를 기다려야 합니다")
	}
}

func TestPreparedCloseContinuesAfterCleanupPanicAndPreservesErrors(t *testing.T) {
	want := errors.New("publisher close failed")
	var order []string
	prepared, err := Prepare(context.Background(), Config{CustomTransports: []core.CustomTransport{
		callbackTransport{stop: func(context.Context) error { order = append(order, "first"); return nil }},
		callbackTransport{stop: func(context.Context) error { order = append(order, "second"); panic("stop panic") }},
	}})
	if err != nil {
		t.Fatal(err)
	}
	prepared.publishers = []publisherCloser{
		{name: "first", close: func(context.Context) error { order = append(order, "publisher-first"); return nil }},
		{name: "second", close: func(context.Context) error { order = append(order, "publisher-second"); return want }},
	}
	err = prepared.Close(context.Background())
	if !errors.Is(err, want) || !strings.Contains(err.Error(), "stop panic") || strings.Join(order, ",") != "second,first,publisher-second,publisher-first" {
		t.Fatalf("나머지 자원 정리와 원인 오류를 보존해야 합니다: %v %v", err, order)
	}
	if next := prepared.Close(context.Background()); next != err {
		t.Fatal("완료한 정리 결과를 보존해야 합니다")
	}
}

func preparationHandler(t *testing.T, a *Application) http.Handler {
	t.Helper()
	h, err := adapter.NetHTTPHandler(a.HTTPAdapter())
	if err != nil {
		t.Fatal(err)
	}
	return h
}
