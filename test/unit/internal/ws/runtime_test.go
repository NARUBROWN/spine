package ws

import (
	"context"
	"crypto/tls"
	"errors"

	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	wsadapter "github.com/NARUBROWN/spine/contract/websocket"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/container"
	"github.com/NARUBROWN/spine/internal/invoker"
	"github.com/NARUBROWN/spine/pkg/boot"
	pkgws "github.com/NARUBROWN/spine/pkg/ws"
)

type rejectingHandshakeInterceptor struct {
	calls      atomic.Int32
	lastHeader string
	lastQuery  string
	lastCookie string
	lastRemote string
	lastHost   string
}

type shutdownGateConnection struct {
	closeOnlyConnection
	entered chan<- struct{}
	release <-chan struct{}
}

func (c *shutdownGateConnection) WriteClose(_ int, _ string, deadline time.Time) error {
	c.entered <- struct{}{}
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	select {
	case <-c.release:
		return nil
	case <-timer.C:
		return context.DeadlineExceeded
	}
}

func TestRuntimeStopClosesConnectionsConcurrently(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered, release := make(chan struct{}, 3), make(chan struct{})
	runtime := &Runtime{ctx: ctx, cancel: cancel, options: normalizedWebSocketOptions{WriteTimeout: 5 * time.Second}, conns: make(map[string]*trackedConn)}
	var connections []*shutdownGateConnection
	for _, id := range []string{"first", "second", "third"} {
		conn := &shutdownGateConnection{entered: entered, release: release}
		connections = append(connections, conn)
		runtime.conns[id] = &trackedConn{conn: conn}
	}
	done := make(chan struct{})
	go func() { runtime.Stop(); close(done) }()
	defer func() {
		close(release)
		<-done
		for _, conn := range connections {
			if conn.closes != 1 {
				t.Errorf("connection Close count = %d", conn.closes)
			}
		}
	}()
	for range connections {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("one stalled close must not prevent the other connections from closing")
		}
	}
	select {
	case <-done:
		t.Fatal("Stop returned before connection cleanup completed")
	default:
	}
}

func (i *rejectingHandshakeInterceptor) BeforeUpgrade(ctx context.Context, request pkgws.Handshake, _ core.HandlerMeta) error {
	i.calls.Add(1)
	i.lastHeader = request.Header("Authorization")
	i.lastQuery = request.Query("token")
	i.lastCookie, _ = request.Cookie("session")
	i.lastRemote = request.RemoteAddr()
	i.lastHost = request.Host()
	return errors.New("unauthorized")
}

type capacityHandshakeInterceptor struct {
	calls        atomic.Int32
	active       atomic.Int32
	maxActive    atomic.Int32
	firstSeen    chan struct{}
	releaseFirst chan struct{}
}

type cancellationHandshakeInterceptor struct {
	started  chan struct{}
	canceled chan struct{}
	release  chan struct{}
}

func (i *cancellationHandshakeInterceptor) BeforeUpgrade(ctx context.Context, request pkgws.Handshake, _ core.HandlerMeta) error {
	close(i.started)
	<-ctx.Done()
	close(i.canceled)
	<-i.release
	return ctx.Err()
}

func (i *capacityHandshakeInterceptor) BeforeUpgrade(context.Context, pkgws.Handshake, core.HandlerMeta) error {
	call := i.calls.Add(1)
	active := i.active.Add(1)
	defer i.active.Add(-1)
	for {
		maximum := i.maxActive.Load()
		if active <= maximum || i.maxActive.CompareAndSwap(maximum, active) {
			break
		}
	}
	if call == 1 {
		close(i.firstSeen)
		<-i.releaseFirst
	}
	return errors.New("unauthorized")
}

type cancellationController struct {
	started  chan struct{}
	canceled chan struct{}
	release  chan struct{}
}

func (c *cancellationController) Wait(ctx context.Context) {
	close(c.started)
	select {
	case <-ctx.Done():
		close(c.canceled)
	case <-c.release:
	}
}

func TestRuntime_StopCancelsAndWaitsForActiveBeforeUpgrade(t *testing.T) {
	interceptor := &cancellationHandshakeInterceptor{
		started:  make(chan struct{}),
		canceled: make(chan struct{}),
		release:  make(chan struct{}),
	}
	runtime, registration := newTestRuntime(t, &cancellationController{}, (*cancellationController).Wait, boot.WebSocketOptions{
		MaxConnections: 1,
	})
	runtime.upgradeInterceptors = []core.WebSocketUpgradeInterceptor{interceptor}

	handled := make(chan struct{})
	go func() {
		recorder := httptest.NewRecorder()
		runtime.HandleConn(recorder, httptest.NewRequest(http.MethodGet, "http://example.test/", nil), registration)
		close(handled)
	}()
	select {
	case <-interceptor.started:
	case <-time.After(time.Second):
		t.Fatal("handshake interceptor가 시작되지 않았습니다")
	}

	stopped := make(chan struct{})
	go func() {
		runtime.Stop()
		close(stopped)
	}()
	select {
	case <-interceptor.canceled:
	case <-time.After(time.Second):
		t.Fatal("Runtime.Stop이 handshake context를 취소하지 않았습니다")
	}
	select {
	case <-stopped:
		t.Fatal("Runtime.Stop은 handshake interceptor가 반환되기 전에 종료되면 안 됩니다")
	case <-time.After(100 * time.Millisecond):
	}

	close(interceptor.release)
	select {
	case <-handled:
	case <-time.After(time.Second):
		t.Fatal("취소된 handshake 요청이 반환되지 않았습니다")
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("handshake 반환 후 Runtime.Stop이 종료되지 않았습니다")
	}
}

func TestRuntime_TrackConnHonorsConnectionLimit(t *testing.T) {
	runtime := &Runtime{
		options: normalizedWebSocketOptions{MaxConnections: 1},
		ctx:     context.Background(),
		conns:   make(map[string]*trackedConn),
	}
	if reserved, _ := runtime.reserveConnectionSlot(); !reserved {
		t.Fatal("first connection should be accepted")
	}
	if reserved, _ := runtime.reserveConnectionSlot(); reserved {
		t.Fatal("connection above the configured limit must be rejected")
	}
}

func TestRuntime_ReleasesCapacityAfterFailedUpgrade(t *testing.T) {
	runtime, registration := newTestRuntime(t, &cancellationController{}, (*cancellationController).Wait, boot.WebSocketOptions{
		MaxConnections: 1,
	})
	defer runtime.Stop()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	runtime.HandleConn(recorder, request, registration)

	if reserved, _ := runtime.reserveConnectionSlot(); !reserved {
		t.Fatal("failed WebSocket upgrade must release its reserved connection slot")
	}
	runtime.releaseConnectionSlot()
}

func TestRuntime_RejectedHandshakeReleasesCapacityAndPreservesRequest(t *testing.T) {
	interceptor := &rejectingHandshakeInterceptor{}
	runtime, registration := newTestRuntime(t, &cancellationController{}, (*cancellationController).Wait, boot.WebSocketOptions{
		MaxConnections: 1,
	})
	runtime.upgradeInterceptors = []core.WebSocketUpgradeInterceptor{interceptor}
	defer runtime.Stop()

	for range 32 {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "http://example.test/?token=query-token", nil)
		request.Header.Set("Authorization", "Bearer header-token")
		request.AddCookie(&http.Cookie{Name: "session", Value: "cookie-token"})
		request.RemoteAddr = "203.0.113.10:4321"
		request.Host = "example.test"
		runtime.HandleConn(recorder, request, registration)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("handshake rejection status = %d, want 401", recorder.Code)
		}
	}

	if runtime.connectionSlots != 0 {
		t.Fatalf("rejected handshakes consumed connection slots: %d", runtime.connectionSlots)
	}
	if reserved, _ := runtime.reserveConnectionSlot(); !reserved {
		t.Fatal("legitimate connection slot must remain available after rejected handshakes")
	}
	runtime.releaseConnectionSlot()
	if interceptor.calls.Load() != 32 || interceptor.lastHeader != "Bearer header-token" || interceptor.lastQuery != "query-token" || interceptor.lastCookie != "cookie-token" || interceptor.lastRemote != "203.0.113.10:4321" || interceptor.lastHost != "example.test" {
		t.Fatalf("handshake request snapshot was incomplete: %+v", interceptor)
	}
}

func TestRuntime_BeforeUpgradeConcurrencyIsBoundedByConnectionCapacity(t *testing.T) {
	interceptor := &capacityHandshakeInterceptor{
		firstSeen:    make(chan struct{}),
		releaseFirst: make(chan struct{}),
	}
	runtime, registration := newTestRuntime(t, &cancellationController{}, (*cancellationController).Wait, boot.WebSocketOptions{
		MaxConnections: 1,
	})
	runtime.upgradeInterceptors = []core.WebSocketUpgradeInterceptor{interceptor}
	defer runtime.Stop()

	firstResult := make(chan int, 1)
	go func() {
		recorder := httptest.NewRecorder()
		runtime.HandleConn(recorder, httptest.NewRequest(http.MethodGet, "http://example.test/", nil), registration)
		firstResult <- recorder.Code
	}()
	select {
	case <-interceptor.firstSeen:
	case <-time.After(time.Second):
		t.Fatal("첫 번째 handshake interceptor가 시작되지 않았습니다")
	}

	secondRecorder := httptest.NewRecorder()
	runtime.HandleConn(secondRecorder, httptest.NewRequest(http.MethodGet, "http://example.test/", nil), registration)
	if secondRecorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("용량을 초과한 handshake는 503이어야 합니다. 실제=%d", secondRecorder.Code)
	}
	if calls := interceptor.calls.Load(); calls != 1 {
		t.Fatalf("용량을 초과한 요청은 handshake interceptor에 진입하면 안 됩니다. 호출=%d", calls)
	}
	if maximum := interceptor.maxActive.Load(); maximum != 1 {
		t.Fatalf("동시 handshake interceptor 실행 수는 1이어야 합니다. 실제=%d", maximum)
	}

	close(interceptor.releaseFirst)
	select {
	case status := <-firstResult:
		if status != http.StatusUnauthorized {
			t.Fatalf("첫 번째 handshake 거부 상태는 401이어야 합니다. 실제=%d", status)
		}
	case <-time.After(time.Second):
		t.Fatal("첫 번째 handshake가 종료되지 않았습니다")
	}
	if runtime.connectionSlots != 0 {
		t.Fatalf("거부된 handshake가 연결 슬롯을 유지했습니다: %d", runtime.connectionSlots)
	}
}

func TestSession_PreservesImmutableHandshakeRequestSnapshot(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://example.test/chat?token=before&role=user", nil)
	request.Header.Set("Authorization", "Bearer before")
	request.AddCookie(&http.Cookie{Name: "session", Value: "cookie-before"})
	request.RemoteAddr = "192.0.2.20:9876"
	originalRequestURI := request.RequestURI
	handshake := newWebSocketRequestSnapshot(request)

	request.Header.Set("Authorization", "Bearer after")
	request.URL.RawQuery = "token=after"
	ctx := handshake

	if ctx.Header("Authorization") != "Bearer before" || ctx.Query("token") != "before" {
		t.Fatalf("header/query snapshot changed: header=%q query=%q", ctx.Header("Authorization"), ctx.Query("token"))
	}
	if cookie, ok := ctx.Cookie("session"); !ok || cookie != "cookie-before" {
		t.Fatalf("cookie snapshot = %q, %t", cookie, ok)
	}
	if ctx.RemoteAddr() != "192.0.2.20:9876" || ctx.Host() != "example.test" || ctx.RequestURI() != originalRequestURI {
		t.Fatalf("remote request snapshot is incomplete: remote=%q host=%q uri=%q", ctx.RemoteAddr(), ctx.Host(), ctx.RequestURI())
	}

	headers := ctx.Headers()
	headers["Authorization"] = []string{"mutated"}
	queries := ctx.Queries()
	queries["token"][0] = "mutated"
	if ctx.Header("Authorization") != "Bearer before" || ctx.Query("token") != "before" {
		t.Fatal("returned header/query maps must not mutate the stored handshake snapshot")
	}
}

func TestIsAllowedWebSocketOrigin_RejectsSameHostCrossSchemeOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "https://example.test/ws", nil)
	req.Host = "example.test"
	req.TLS = &tls.ConnectionState{}
	req.Header.Set("Origin", "http://example.test")
	if isAllowedWebSocketOriginWithTrustedProxies(req, nil, nil) {
		t.Fatal("secure endpoint must reject a same-host HTTP origin")
	}
	req.Header.Set("Origin", "https://example.test")
	if !isAllowedWebSocketOriginWithTrustedProxies(req, nil, nil) {
		t.Fatal("secure endpoint must allow an exact same-origin request")
	}
}

func TestIsAllowedWebSocketOrigin_TrustsForwardedProtoOnlyFromConfiguredProxy(t *testing.T) {
	_, trustedNetwork, err := net.ParseCIDR("10.0.0.0/8")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://example.test/ws", nil)
	req.Host = "example.test"
	req.Header.Set("Origin", "https://example.test")
	req.Header.Set("Forwarded", "for=192.0.2.10;proto=https;host=example.test")

	req.RemoteAddr = "203.0.113.5:1234"
	if isAllowedWebSocketOriginWithTrustedProxies(req, nil, []*net.IPNet{trustedNetwork}) {
		t.Fatal("forwarding headers from an untrusted peer must be ignored")
	}

	req.RemoteAddr = "10.1.2.3:1234"
	if !isAllowedWebSocketOriginWithTrustedProxies(req, nil, []*net.IPNet{trustedNetwork}) {
		t.Fatal("Forwarded proto from a trusted proxy must determine the public request scheme")
	}

	req.Header.Del("Forwarded")
	req.Header.Set("X-Forwarded-Proto", "https")
	if !isAllowedWebSocketOriginWithTrustedProxies(req, nil, []*net.IPNet{trustedNetwork}) {
		t.Fatal("X-Forwarded-Proto from a trusted proxy must be used as a fallback")
	}
}

func TestIsAllowedWebSocketOrigin_ExplicitAllowedOriginDoesNotRequireTrustedProxy(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://internal.test/ws", nil)
	req.Host = "internal.test"
	req.RemoteAddr = "203.0.113.5:1234"
	req.Header.Set("Origin", "https://app.example.test")
	if !isAllowedWebSocketOriginWithTrustedProxies(req, []string{"https://app.example.test"}, nil) {
		t.Fatal("explicit AllowedOrigins must remain authoritative")
	}
}

func newTestRuntime(t *testing.T, controller any, handler any, options boot.WebSocketOptions) (*Runtime, Registration) {
	t.Helper()

	registry := NewRegistry()
	if err := registry.Register("/", handler); err != nil {
		t.Fatalf("WebSocket 등록 실패: %v", err)
	}
	registration := registry.Registrations()[0]

	c := container.New()
	switch typed := controller.(type) {
	case *cancellationController:
		_ = c.RegisterConstructor(func() *cancellationController { return typed })
	default:
		t.Fatalf("지원하지 않는 테스트 컨트롤러: %T", controller)
	}

	return NewRuntime(invoker.NewInvoker(c), failingUpgradeAdapter{}, options, nil), registration
}

type failingUpgradeAdapter struct{}

func (failingUpgradeAdapter) Upgrade(w http.ResponseWriter, _ *http.Request) (wsadapter.Connection, error) {
	http.Error(w, "upgrade rejected by test adapter", http.StatusBadRequest)
	return nil, errors.New("upgrade rejected by test adapter")
}

type handshakeProbe struct {
	run func(context.Context, pkgws.Handshake) error
}

func (p *handshakeProbe) BeforeUpgrade(ctx context.Context, request pkgws.Handshake, _ core.HandlerMeta) error {
	return p.run(ctx, request)
}
func TestBeforeUpgradeReceivesScopedContextAndSnapshot(t *testing.T) {
	for _, outcome := range []string{"success", "error", "panic"} {
		t.Run(outcome, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			read := false
			var hookCtx context.Context
			writer := &handshakeProbe{run: func(got context.Context, request pkgws.Handshake) error {
				hookCtx = got
				if got == ctx || got.Err() != nil || request.Path() != "/chat" || request.Header("X-Request-ID") != "handshake" {
					t.Fatal("incorrect handshake inputs")
				}
				request.Headers()["X-Request-ID"] = []string{"changed"}
				switch outcome {
				case "error":
					return errors.New("rejected")
				case "panic":
					panic("handshake panic")
				}
				return nil
			}}
			reader := &handshakeProbe{run: func(got context.Context, request pkgws.Handshake) error {
				read = true
				if got != hookCtx || got.Err() != nil || request.Header("X-Request-ID") != "handshake" {
					t.Fatal("shared snapshot was mutated")
				}
				return nil
			}}
			runtime := &Runtime{upgradeInterceptors: []core.WebSocketUpgradeInterceptor{writer, reader}}
			request := httptest.NewRequest(http.MethodGet, "/chat", nil)
			request.Header.Set("X-Request-ID", "handshake")
			err := runtime.beforeUpgrade(ctx, newWebSocketRequestSnapshot(request), Registration{})
			if (err != nil) != (outcome != "success") || read != (outcome == "success") {
				t.Fatalf("err=%v read=%v", err, read)
			}
			if hookCtx.Err() != context.Canceled || ctx.Err() != nil {
				t.Fatal("업그레이드 검사 컨텍스트만 취소되어야 합니다")
			}
		})
	}
}

type upgradeProbeAdapter struct {
	run func(*http.Request)
}

func (a upgradeProbeAdapter) Upgrade(_ http.ResponseWriter, req *http.Request) (wsadapter.Connection, error) {
	a.run(req)
	return nil, errors.New("검사용 업그레이드 종료")
}

func TestUpgradeHooksFinishBeforeAdapterStarts(t *testing.T) {
	for _, deny := range []bool{false, true} {
		name := "허용"
		if deny {
			name = "거부"
		}
		t.Run(name, func(t *testing.T) {
			var hookCtx, adapterCtx context.Context
			globalCalls, routeCalls := 0, 0
			global := &handshakeProbe{run: func(ctx context.Context, _ pkgws.Handshake) error {
				globalCalls++
				hookCtx = ctx
				return nil
			}}
			route := &handshakeProbe{run: func(ctx context.Context, _ pkgws.Handshake) error {
				routeCalls++
				if globalCalls != 1 || ctx != hookCtx || ctx.Err() != nil {
					t.Fatal("경로 훅은 전역 훅 뒤에 같은 컨텍스트로 실행돼야 합니다")
				}
				if deny {
					return errors.New("연결 거부")
				}
				return nil
			}}
			adapter := upgradeProbeAdapter{run: func(req *http.Request) {
				adapterCtx = req.Context()
				if routeCalls != 1 || hookCtx.Err() != context.Canceled || adapterCtx.Err() != nil {
					t.Fatal("검사 컨텍스트가 종료된 뒤 어댑터가 활성 컨텍스트로 실행돼야 합니다")
				}
			}}
			runtime := NewRuntime(invoker.NewInvoker(container.New()), adapter, boot.WebSocketOptions{}, nil, global)
			defer runtime.Stop()
			response := httptest.NewRecorder()
			runtime.HandleConn(response, httptest.NewRequest(http.MethodGet, "/chat", nil), Registration{
				Path: "/chat", UpgradeInterceptors: []core.WebSocketUpgradeInterceptor{route},
			})
			if globalCalls != 1 || routeCalls != 1 || hookCtx.Err() != context.Canceled {
				t.Fatal("연결 검사 훅의 수명이 요청에 맞지 않습니다")
			}
			if deny {
				if adapterCtx != nil || response.Code != http.StatusUnauthorized {
					t.Fatal("거부된 연결에서 어댑터를 호출했습니다")
				}
			} else if adapterCtx == nil || adapterCtx.Err() != context.Canceled {
				t.Fatal("어댑터 컨텍스트가 완료 시점에 취소되지 않았습니다")
			}
		})
	}
}
