package ws

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/container"
	"github.com/NARUBROWN/spine/internal/invoker"
	"github.com/NARUBROWN/spine/internal/pipeline"
	"github.com/NARUBROWN/spine/internal/resolver"
	spinerouter "github.com/NARUBROWN/spine/internal/router"
	"github.com/NARUBROWN/spine/pkg/boot"
	pkgws "github.com/NARUBROWN/spine/pkg/ws"
	"github.com/gorilla/websocket"
)

type rejectingHandshakeInterceptor struct {
	calls      atomic.Int32
	lastHeader string
	lastQuery  string
	lastCookie string
	lastRemote string
	lastHost   string
}

func (i *rejectingHandshakeInterceptor) PreHandshake(ctx core.WebSocketHandshakeContext, _ core.HandlerMeta) error {
	i.calls.Add(1)
	i.lastHeader = ctx.Header("Authorization")
	i.lastQuery = ctx.Query("token")
	i.lastCookie, _ = ctx.Cookie("session")
	i.lastRemote = ctx.RemoteAddr()
	i.lastHost = ctx.Host()
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

func (i *cancellationHandshakeInterceptor) PreHandshake(ctx core.WebSocketHandshakeContext, _ core.HandlerMeta) error {
	close(i.started)
	<-ctx.Context().Done()
	close(i.canceled)
	<-i.release
	return ctx.Context().Err()
}

func (i *capacityHandshakeInterceptor) PreHandshake(core.WebSocketHandshakeContext, core.HandlerMeta) error {
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

type requestAwareInterceptor struct {
	handshakeSeen chan struct{}
	messageSeen   chan struct{}
	handshakeAuth string
	messageAuth   string
	messageQuery  string
	messageCookie string
	messageRemote string
}

func (i *requestAwareInterceptor) PreHandshake(ctx core.WebSocketHandshakeContext, _ core.HandlerMeta) error {
	i.handshakeAuth = ctx.Header("Authorization")
	close(i.handshakeSeen)
	return nil
}

func (i *requestAwareInterceptor) PreHandle(ctx core.ExecutionContext, _ core.HandlerMeta) error {
	i.messageAuth = ctx.Header("Authorization")
	i.messageQuery = ctx.Queries()["token"][0]
	if requestCtx, ok := ctx.(core.WebSocketMessageContext); ok {
		i.messageCookie, _ = requestCtx.Cookie("session")
		i.messageRemote = requestCtx.RemoteAddr()
	}
	close(i.messageSeen)
	return nil
}

func (*requestAwareInterceptor) PostHandle(core.ExecutionContext, core.HandlerMeta) {}
func (*requestAwareInterceptor) BeforeResponse(core.ExecutionContext, core.HandlerMeta, error) error {
	return nil
}
func (*requestAwareInterceptor) AfterCompletion(core.ExecutionContext, core.HandlerMeta, error) {}

type cancellationController struct {
	started  chan struct{}
	canceled chan struct{}
	release  chan struct{}
}

type stopWaitController struct {
	started  chan struct{}
	canceled chan struct{}
	release  chan struct{}
	finished chan struct{}
}

func (c *stopWaitController) Wait(ctx context.Context) {
	close(c.started)
	<-ctx.Done()
	close(c.canceled)
	<-c.release
	close(c.finished)
}

func (c *cancellationController) Wait(ctx context.Context) {
	close(c.started)
	select {
	case <-ctx.Done():
		close(c.canceled)
	case <-c.release:
	}
}

type concurrentSendController struct {
	errs chan error
}

func (c *concurrentSendController) Burst(ctx context.Context) {
	const messages = 32
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range messages {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			if err := pkgws.Send(ctx, pkgws.TextMessage, []byte(fmt.Sprintf("message-%d", index))); err != nil {
				select {
				case c.errs <- err:
				default:
				}
			}
		}(i)
	}
	close(start)
	wg.Wait()
	close(c.errs)
}

func TestRuntime_StopCancelsActiveHandlerAndRejectsNewConnections(t *testing.T) {
	controller := &cancellationController{
		started:  make(chan struct{}),
		canceled: make(chan struct{}),
		release:  make(chan struct{}),
	}
	runtime, registration := newTestRuntime(t, controller, (*cancellationController).Wait, boot.WebSocketOptions{
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
		PingInterval: time.Second,
	})
	defer close(controller.release)

	server := newRuntimeTestServer(runtime, registration)
	defer server.Close()

	conn := dialRuntimeTestServer(t, server)
	defer conn.Close()
	if err := conn.WriteMessage(websocket.TextMessage, []byte("start")); err != nil {
		t.Fatalf("메시지 전송 실패: %v", err)
	}
	select {
	case <-controller.started:
	case <-time.After(time.Second):
		t.Fatal("WebSocket 핸들러가 시작되지 않았습니다")
	}

	runtime.Stop()

	select {
	case <-controller.canceled:
	case <-time.After(time.Second):
		t.Fatal("Runtime.Stop이 활성 핸들러의 context를 취소하지 않았습니다")
	}

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	_, response, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("중지된 런타임은 신규 연결을 거부해야 합니다")
	}
	if response == nil || response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("중지된 런타임은 503을 반환해야 합니다: %v", response)
	}
}

func TestRuntime_StopWaitsForActiveHandlerToReturn(t *testing.T) {
	controller := &stopWaitController{
		started:  make(chan struct{}),
		canceled: make(chan struct{}),
		release:  make(chan struct{}),
		finished: make(chan struct{}),
	}
	runtime, registration := newTestRuntime(t, controller, (*stopWaitController).Wait, boot.WebSocketOptions{
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
		PingInterval: time.Second,
	})

	server := newRuntimeTestServer(runtime, registration)
	defer server.Close()
	conn := dialRuntimeTestServer(t, server)
	defer conn.Close()
	if err := conn.WriteMessage(websocket.TextMessage, []byte("start")); err != nil {
		t.Fatalf("메시지 전송 실패: %v", err)
	}
	select {
	case <-controller.started:
	case <-time.After(time.Second):
		t.Fatal("WebSocket 핸들러가 시작되지 않았습니다")
	}

	stopped := make(chan struct{})
	go func() {
		runtime.Stop()
		close(stopped)
	}()
	select {
	case <-controller.canceled:
	case <-time.After(time.Second):
		t.Fatal("Runtime.Stop이 활성 핸들러의 context를 취소하지 않았습니다")
	}
	select {
	case <-stopped:
		t.Fatal("Runtime.Stop은 활성 핸들러가 반환되기 전에 종료되면 안 됩니다")
	case <-time.After(100 * time.Millisecond):
	}

	close(controller.release)
	select {
	case <-controller.finished:
	case <-time.After(time.Second):
		t.Fatal("취소된 WebSocket 핸들러가 반환되지 않았습니다")
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("활성 핸들러 반환 후 Runtime.Stop이 종료되지 않았습니다")
	}
}

func TestRuntime_StopCancelsAndWaitsForActivePreHandshake(t *testing.T) {
	interceptor := &cancellationHandshakeInterceptor{
		started:  make(chan struct{}),
		canceled: make(chan struct{}),
		release:  make(chan struct{}),
	}
	runtime, registration := newTestRuntime(t, &cancellationController{}, (*cancellationController).Wait, boot.WebSocketOptions{
		MaxConnections: 1,
	})
	runtime.handshakeInterceptors = []core.WebSocketHandshakeInterceptor{interceptor}

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

func TestRuntime_ConcurrentSendsRemainSafeWhilePingRuns(t *testing.T) {
	controller := &concurrentSendController{errs: make(chan error, 1)}
	runtime, registration := newTestRuntime(t, controller, (*concurrentSendController).Burst, boot.WebSocketOptions{
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
		PingInterval: time.Millisecond,
	})
	defer runtime.Stop()

	server := newRuntimeTestServer(runtime, registration)
	defer server.Close()
	conn := dialRuntimeTestServer(t, server)
	defer conn.Close()

	if err := conn.WriteMessage(websocket.TextMessage, []byte("burst")); err != nil {
		t.Fatalf("메시지 전송 실패: %v", err)
	}

	seen := make(map[string]struct{}, 32)
	for len(seen) < 32 {
		if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			t.Fatalf("read deadline 설정 실패: %v", err)
		}
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("동시 전송 응답 수신 실패 (%d/32): %v", len(seen), err)
		}
		if messageType != websocket.TextMessage {
			t.Fatalf("예상하지 못한 메시지 타입: %d", messageType)
		}
		seen[string(payload)] = struct{}{}
	}
	for {
		select {
		case err, ok := <-controller.errs:
			if !ok {
				return
			}
			if err != nil {
				t.Fatalf("동시 전송 실패: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("동시 WebSocket 전송 결과 대기가 시간 내 종료되지 않았습니다")
		}
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

func TestRuntime_RejectsCapacityBeforeWebSocketUpgrade(t *testing.T) {
	controller := &cancellationController{
		started:  make(chan struct{}),
		canceled: make(chan struct{}),
		release:  make(chan struct{}),
	}
	runtime, registration := newTestRuntime(t, controller, (*cancellationController).Wait, boot.WebSocketOptions{
		MaxConnections:     1,
		CapacityRetryAfter: 3 * time.Second,
	})
	defer runtime.Stop()
	defer close(controller.release)

	server := newRuntimeTestServer(runtime, registration)
	defer server.Close()
	first := dialRuntimeTestServer(t, server)
	defer first.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	_, response, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("connection above capacity must be rejected")
	}
	if response == nil || response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("capacity rejection must return HTTP 503 before upgrade: %v", response)
	}
	defer response.Body.Close()
	if got := response.Header.Get("Retry-After"); got != "3" {
		t.Fatalf("Retry-After = %q, want 3", got)
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
	runtime.handshakeInterceptors = []core.WebSocketHandshakeInterceptor{interceptor}
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

func TestRuntime_PreHandshakeConcurrencyIsBoundedByConnectionCapacity(t *testing.T) {
	interceptor := &capacityHandshakeInterceptor{
		firstSeen:    make(chan struct{}),
		releaseFirst: make(chan struct{}),
	}
	runtime, registration := newTestRuntime(t, &cancellationController{}, (*cancellationController).Wait, boot.WebSocketOptions{
		MaxConnections: 1,
	})
	runtime.handshakeInterceptors = []core.WebSocketHandshakeInterceptor{interceptor}
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

func TestRuntime_CommonInterceptorReadsHandshakeCredentialsFromMessageContext(t *testing.T) {
	controller := &cancellationController{
		started:  make(chan struct{}),
		canceled: make(chan struct{}),
		release:  make(chan struct{}),
	}
	interceptor := &requestAwareInterceptor{
		handshakeSeen: make(chan struct{}),
		messageSeen:   make(chan struct{}),
	}
	runtime, registration := newTestRuntime(t, controller, (*cancellationController).Wait, boot.WebSocketOptions{})
	runtime.pipeline.AddInterceptor(interceptor)
	runtime.handshakeInterceptors = []core.WebSocketHandshakeInterceptor{interceptor}
	defer runtime.Stop()

	server := newRuntimeTestServer(runtime, registration)
	defer server.Close()
	defer close(controller.release)
	headers := http.Header{}
	headers.Set("Authorization", "Bearer shared-token")
	headers.Set("Cookie", "session=cookie-token")
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "?token=query-token"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, headers)
	if err != nil {
		t.Fatalf("WebSocket connection failed: %v", err)
	}
	defer conn.Close()
	select {
	case <-interceptor.handshakeSeen:
	case <-time.After(time.Second):
		t.Fatal("optional handshake interceptor did not run before upgrade")
	}
	if err := conn.WriteMessage(websocket.TextMessage, []byte("authenticate")); err != nil {
		t.Fatalf("message send failed: %v", err)
	}
	select {
	case <-interceptor.messageSeen:
	case <-time.After(time.Second):
		t.Fatal("common message interceptor did not run")
	}
	if interceptor.handshakeAuth != "Bearer shared-token" || interceptor.messageAuth != "Bearer shared-token" || interceptor.messageQuery != "query-token" || interceptor.messageCookie != "cookie-token" || interceptor.messageRemote == "" {
		t.Fatalf("credentials were not preserved across handshake/message contexts: %+v", interceptor)
	}
}

func TestWSExecutionContext_PreservesImmutableHandshakeRequestSnapshot(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://example.test/chat?token=before&role=user", nil)
	request.Header.Set("Authorization", "Bearer before")
	request.AddCookie(&http.Cookie{Name: "session", Value: "cookie-before"})
	request.RemoteAddr = "192.0.2.20:9876"
	originalRequestURI := request.RequestURI
	handshake := newWebSocketRequestSnapshot(request)

	messageContext := NewWSExecutionContext(context.Background(), "conn", "/chat", websocket.TextMessage, []byte("payload"), nil, func(int, []byte) error { return nil }, handshake)
	request.Header.Set("Authorization", "Bearer after")
	request.URL.RawQuery = "token=after"

	ctx, ok := messageContext.(core.WebSocketMessageContext)
	if !ok {
		t.Fatalf("message context = %T, want core.WebSocketMessageContext", messageContext)
	}
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
	case *concurrentSendController:
		_ = c.RegisterConstructor(func() *concurrentSendController { return typed })
	case *stopWaitController:
		_ = c.RegisterConstructor(func() *stopWaitController { return typed })
	default:
		t.Fatalf("지원하지 않는 테스트 컨트롤러: %T", controller)
	}

	router := spinerouter.NewRouter()
	router.Register("WS", registration.Path, registration.Meta)
	p := pipeline.NewPipeline(router, invoker.NewInvoker(c))
	p.AddArgumentResolver(&resolver.StdContextResolver{})

	return NewRuntime(p, options), registration
}

func newRuntimeTestServer(runtime *Runtime, registration Registration) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		runtime.HandleConn(w, req, registration)
	}))
}

func dialRuntimeTestServer(t *testing.T, server *httptest.Server) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WebSocket 연결 실패: %v", err)
	}
	return conn
}
