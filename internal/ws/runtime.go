package ws

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	validation "github.com/NARUBROWN/spine/contract/validator"
	wsadapter "github.com/NARUBROWN/spine/contract/websocket"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/configuration"
	"github.com/NARUBROWN/spine/internal/contextview"
	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/httpfailure"
	"github.com/NARUBROWN/spine/internal/pipeline"
)

const (
	defaultWSHandshakeTimeout = 10 * time.Second
	defaultWSReadTimeout      = 60 * time.Second
	defaultWSWriteTimeout     = 10 * time.Second
	defaultWSMaxMessageBytes  = 1 << 20
	defaultWSCapacityRetry    = 5 * time.Second
)

type normalizedWebSocketOptions struct {
	AllowedOrigins   []string
	TrustedProxies   []*net.IPNet
	MaxConnections   int
	CapacityRetry    time.Duration
	MaxMessageBytes  int64
	HandshakeTimeout time.Duration
	ReadTimeout      time.Duration
	WriteTimeout     time.Duration
	PingInterval     time.Duration
}

type Runtime struct {
	pipeline *pipeline.Pipeline
	adapter  wsadapter.Adapter
	options  normalizedWebSocketOptions
	stopOnce sync.Once
	ctx      context.Context
	cancel   context.CancelFunc
	// lifecycleMu는 HandleConn 등록과 Stop의 대기 시작을 직렬화합니다.
	// Stop이 시작된 뒤에는 WaitGroup에 새로운 작업이 추가되지 않습니다.
	lifecycleMu sync.Mutex
	stopping    bool
	handlers    sync.WaitGroup
	connMu      sync.Mutex
	conns       map[string]*trackedConn
	// connectionSlots는 활성 연결과 진행 중인 핸드셰이크를 모두 포함합니다.
	// 동시에 여러 핸드셰이크가 진행되어도 최대 연결 수를 넘지 않도록
	// 연결을 업그레이드하기 전에 슬롯을 미리 확보합니다.
	connectionSlots       int
	handshakeInterceptors []core.WebSocketHandshakeInterceptor
}

type trackedConn struct {
	conn wsadapter.Connection
	mu   sync.Mutex
}

func writeDeadline(timeout time.Duration) time.Time {
	if timeout > 0 {
		return time.Now().Add(timeout)
	}
	return time.Time{}
}

func (c *trackedConn) writeMessage(messageType int, data []byte, timeout time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteMessage(messageType, data, writeDeadline(timeout))
}

func (c *trackedConn) ping(timeout time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.Ping(writeDeadline(timeout))
}

func (c *trackedConn) writeClose(code int, reason string, timeout time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteClose(code, reason, writeDeadline(timeout))
}

func NewRuntime(pipeline *pipeline.Pipeline, adapter wsadapter.Adapter, opts configuration.WebSocketOptions, handshakeInterceptors ...core.WebSocketHandshakeInterceptor) *Runtime {
	if wsadapter.ValidateAdapter(adapter) != nil {
		panic("ws: adapter cannot be nil")
	}
	if pipeline == nil {
		panic("ws: pipeline cannot be nil")
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Runtime{
		pipeline:              pipeline,
		adapter:               adapter,
		options:               normalizeWebSocketOptions(opts),
		ctx:                   ctx,
		cancel:                cancel,
		conns:                 make(map[string]*trackedConn),
		handshakeInterceptors: append([]core.WebSocketHandshakeInterceptor(nil), handshakeInterceptors...),
	}
}

// AdapterOptions는 커널 기본값을 적용한 연결 설정만 플랫폼에 전달합니다.
func AdapterOptions(opts configuration.WebSocketOptions) wsadapter.Options {
	normalized := normalizeWebSocketOptions(opts)
	return wsadapter.Options{HandshakeTimeout: normalized.HandshakeTimeout, ReadTimeout: normalized.ReadTimeout, MaxMessageBytes: normalized.MaxMessageBytes}
}

func normalizeWebSocketOptions(opts configuration.WebSocketOptions) normalizedWebSocketOptions {
	normalized := normalizedWebSocketOptions{
		AllowedOrigins:   append([]string(nil), opts.AllowedOrigins...),
		MaxConnections:   opts.MaxConnections,
		CapacityRetry:    opts.CapacityRetryAfter,
		MaxMessageBytes:  opts.MaxMessageBytes,
		HandshakeTimeout: opts.HandshakeTimeout,
		ReadTimeout:      opts.ReadTimeout,
		WriteTimeout:     opts.WriteTimeout,
		PingInterval:     opts.PingInterval,
	}
	for _, cidr := range opts.TrustedProxyCIDRs {
		_, network, err := net.ParseCIDR(strings.TrimSpace(cidr))
		if err == nil {
			normalized.TrustedProxies = append(normalized.TrustedProxies, network)
		}
	}

	if normalized.MaxMessageBytes == 0 {
		normalized.MaxMessageBytes = defaultWSMaxMessageBytes
	}
	if normalized.MaxConnections == 0 {
		normalized.MaxConnections = configuration.DefaultWebSocketMaxConnections
	}
	if normalized.CapacityRetry == 0 {
		normalized.CapacityRetry = defaultWSCapacityRetry
	}
	if normalized.HandshakeTimeout == 0 {
		normalized.HandshakeTimeout = defaultWSHandshakeTimeout
	}
	if normalized.ReadTimeout == 0 {
		normalized.ReadTimeout = defaultWSReadTimeout
	}
	if normalized.WriteTimeout == 0 {
		normalized.WriteTimeout = defaultWSWriteTimeout
	}
	if normalized.PingInterval == 0 {
		normalized.PingInterval = normalized.ReadTimeout / 2
	}
	if normalized.PingInterval <= 0 {
		normalized.PingInterval = 30 * time.Second
	}

	return normalized
}

func (r *Runtime) HandleConn(w http.ResponseWriter, req *http.Request, reg Registration) {
	if !r.beginHandleConn() {
		http.Error(w, "websocket runtime is shutting down", http.StatusServiceUnavailable)
		return
	}
	defer r.handlers.Done()

	reserved, slots := r.reserveConnectionSlot()
	if !reserved {
		select {
		case <-r.ctx.Done():
			http.Error(w, "websocket runtime is shutting down", http.StatusServiceUnavailable)
		default:
			retryAfterSeconds := int64(r.options.CapacityRetry / time.Second)
			if r.options.CapacityRetry%time.Second != 0 {
				retryAfterSeconds++
			}
			if retryAfterSeconds < 1 {
				retryAfterSeconds = 1
			}
			w.Header().Set("Retry-After", strconv.FormatInt(retryAfterSeconds, 10))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("{\"code\":\"WEBSOCKET_CAPACITY_EXCEEDED\",\"message\":\"WebSocket connection capacity has been reached.\"}\n"))
			log.Printf("[WS] Connection rejected: capacity reached (path=%s, active_and_pending=%d, limit=%d)", reg.Path, slots, r.options.MaxConnections)
		}
		return
	}
	slotOwnedByHandler := true
	defer func() {
		if slotOwnedByHandler {
			r.releaseConnectionSlot()
		}
	}()

	handshakeCtx, cancelHandshake := context.WithCancel(req.Context())
	stopHandshakeCancellation := context.AfterFunc(r.ctx, cancelHandshake)
	defer func() {
		stopHandshakeCancellation()
		cancelHandshake()
	}()
	handshakeContext := newWebSocketRequestSnapshot(req.WithContext(handshakeCtx))
	if err := r.preHandshake(handshakeContext, reg); err != nil {
		writeHandshakeRejection(w, err)
		log.Printf("[WS] Handshake rejected (path=%s): %v", reg.Path, err)
		return
	}

	if handshakeCtx.Err() != nil || r.ctx.Err() != nil {
		http.Error(w, "websocket runtime is shutting down", http.StatusServiceUnavailable)
		return
	}
	if !isAllowedWebSocketOriginWithTrustedProxies(req, r.options.AllowedOrigins, r.options.TrustedProxies) {
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}
	conn, err := r.adapter.Upgrade(w, req.WithContext(handshakeCtx))
	if err != nil {
		if wsadapter.ValidateConnection(conn) == nil {
			_ = conn.Close()
		}
		log.Printf("[WS] Upgrade failed (%s): %v", reg.Path, err)
		return
	}
	if err := wsadapter.ValidateConnection(conn); err != nil {
		log.Printf("[WS] Upgrade failed (%s): %v", reg.Path, err)
		return
	}

	connID := generateConnID()
	tracked := &trackedConn{conn: conn}
	if !r.activateReservedConnection(connID, tracked) {
		_ = conn.Close()
		return
	}
	slotOwnedByHandler = false
	connCtx, cancelConn := context.WithCancel(req.Context())
	stopRuntimeCancellation := context.AfterFunc(r.ctx, cancelConn)
	defer func() {
		stopRuntimeCancellation()
		cancelConn()
		r.untrackConn(connID)
		_ = conn.Close()
	}()

	log.Printf("[WS] Connection established (conn=%p, path=%s)", &connID, reg.Path)

	sendFn := func(messageType int, data []byte) error {
		return tracked.writeMessage(messageType, data, r.options.WriteTimeout)
	}

	done := make(chan struct{})
	defer close(done)

	go func() {
		ticker := time.NewTicker(r.options.PingInterval)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-connCtx.Done():
				return
			case <-ticker.C:
				err := tracked.ping(r.options.WriteTimeout)
				if err != nil {
					return
				}
			}
		}
	}()

	// 연결당 루프
	for {
		msgType, payload, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[WS] Connection closed (conn=%p): %v", &connID, err)
			return
		}

		ctx := NewWSExecutionContext(
			connCtx,
			connID,
			req.URL.Path,
			msgType,
			payload,
			nil,
			sendFn,
			handshakeContext,
		)

		if err := r.pipeline.ExecuteRegistered(ctx, reg.Meta); err != nil {
			log.Printf("[WS] Handler failed (conn=%p): %v", &connID, err)
			code, reason := wsadapter.CloseInternalServerErr, "handler error"
			if _, ok := validation.AsError(err); ok {
				code, reason = wsadapter.ClosePolicyViolation, "invalid message"
			}
			_ = tracked.writeClose(
				code, reason,
				r.options.WriteTimeout,
			)
			return
		}
	}
}

func (r *Runtime) preHandshake(ctx execution.WebSocketRequestContext, reg Registration) error {
	executionCtx := NewWSExecutionContext(ctx.Context(), "", ctx.Path(), 0, nil, nil, nil, ctx)
	defer executionCtx.(interface{ ReleaseStore() }).ReleaseStore()
	publicCtx := execution.NewContext(executionCtx)
	for _, interceptor := range r.handshakeInterceptors {
		if interceptor == nil {
			continue
		}
		if err := callPreHandshake(interceptor, publicCtx, reg.Meta); err != nil {
			return err
		}
	}
	for _, interceptor := range reg.Meta.Interceptors {
		handshakeInterceptor, ok := interceptor.(core.WebSocketHandshakeInterceptor)
		if !ok {
			continue
		}
		if err := callPreHandshake(handshakeInterceptor, publicCtx, reg.Meta); err != nil {
			return err
		}
	}
	return nil
}

func callPreHandshake(interceptor core.WebSocketHandshakeInterceptor, ctx contextview.Ctx, meta execution.HandlerMeta) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("websocket handshake interceptor panic: %v", recovered)
		}
	}()
	return interceptor.PreHandshake(ctx, meta.View())
}

func writeHandshakeRejection(w http.ResponseWriter, err error) {
	status := http.StatusUnauthorized
	var httpErr *httpfailure.HTTPError
	if errors.As(err, &httpErr) && httpErr.Status >= 400 && httpErr.Status <= 499 {
		status = httpErr.Status
	}
	http.Error(w, http.StatusText(status), status)
}

func (r *Runtime) Stop() {
	r.stopOnce.Do(func() {
		r.lifecycleMu.Lock()
		r.stopping = true
		r.cancel()
		r.lifecycleMu.Unlock()

		r.connMu.Lock()
		conns := make(map[string]*trackedConn, len(r.conns))
		for id, conn := range r.conns {
			conns[id] = conn
		}
		r.connMu.Unlock()

		var closing sync.WaitGroup
		panics := make(chan any, len(conns))
		for _, conn := range conns {
			closing.Add(1)
			go func(conn *trackedConn) {
				defer closing.Done()
				defer func() {
					if value := recover(); value != nil {
						panics <- value
					}
				}()
				// WriteClose가 실패하거나 panic해도 자원 해제는 시도합니다.
				defer conn.conn.Close()
				_ = conn.writeClose(
					wsadapter.CloseNormalClosure, "server shutting down",
					r.options.WriteTimeout,
				)
			}(conn)
		}
		closing.Wait()
		close(panics)
		// 다른 연결의 정리까지 마친 뒤 기존 앱 cleanup recover 경계로 전달합니다.
		for value := range panics {
			panic(value)
		}

		r.handlers.Wait()
		log.Printf("[WS] WebSocket runtime stopped")
	})
}

func (r *Runtime) beginHandleConn() bool {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()
	if r.stopping {
		return false
	}
	r.handlers.Add(1)
	return true
}

func (r *Runtime) reserveConnectionSlot() (bool, int) {
	r.connMu.Lock()
	defer r.connMu.Unlock()

	select {
	case <-r.ctx.Done():
		return false, r.connectionSlots
	default:
		if r.options.MaxConnections > 0 && r.connectionSlots >= r.options.MaxConnections {
			return false, r.connectionSlots
		}
		r.connectionSlots++
		return true, r.connectionSlots
	}
}

func (r *Runtime) activateReservedConnection(connID string, conn *trackedConn) bool {
	r.connMu.Lock()
	defer r.connMu.Unlock()
	select {
	case <-r.ctx.Done():
		return false
	default:
		r.conns[connID] = conn
		return true
	}
}

func (r *Runtime) releaseConnectionSlot() {
	r.connMu.Lock()
	defer r.connMu.Unlock()
	if r.connectionSlots > 0 {
		r.connectionSlots--
	}
}

func (r *Runtime) untrackConn(connID string) {
	r.connMu.Lock()
	defer r.connMu.Unlock()
	if _, ok := r.conns[connID]; ok {
		delete(r.conns, connID)
		if r.connectionSlots > 0 {
			r.connectionSlots--
		}
	}
}

func isAllowedWebSocketOriginWithTrustedProxies(req *http.Request, allowedOrigins []string, trustedProxies []*net.IPNet) bool {
	origin := req.Header.Get("Origin")
	if origin == "" {
		return true
	}

	for _, allowed := range allowedOrigins {
		if allowed == "*" || allowed == origin {
			return true
		}
	}

	if len(allowedOrigins) > 0 {
		return false
	}

	originURL, err := url.Parse(origin)
	if err != nil {
		return false
	}

	if !strings.EqualFold(originURL.Host, req.Host) {
		return false
	}

	requestScheme := "http"
	if req.TLS != nil {
		requestScheme = "https"
	} else if forwardedScheme := trustedForwardedScheme(req, trustedProxies); forwardedScheme != "" {
		requestScheme = forwardedScheme
	}
	return strings.EqualFold(originURL.Scheme, requestScheme)
}

func trustedForwardedScheme(req *http.Request, trustedProxies []*net.IPNet) string {
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		host = req.RemoteAddr
	}
	peerIP := net.ParseIP(strings.TrimSpace(host))
	if peerIP == nil {
		return ""
	}
	trusted := false
	for _, network := range trustedProxies {
		if network != nil && network.Contains(peerIP) {
			trusted = true
			break
		}
	}
	if !trusted {
		return ""
	}

	for _, forwarded := range req.Header.Values("Forwarded") {
		firstHop := strings.SplitN(forwarded, ",", 2)[0]
		for _, parameter := range strings.Split(firstHop, ";") {
			name, value, ok := strings.Cut(strings.TrimSpace(parameter), "=")
			if ok && strings.EqualFold(name, "proto") {
				if scheme := normalizedForwardedScheme(value); scheme != "" {
					return scheme
				}
			}
		}
	}

	if forwardedProto := req.Header.Get("X-Forwarded-Proto"); forwardedProto != "" {
		return normalizedForwardedScheme(strings.SplitN(forwardedProto, ",", 2)[0])
	}
	return ""
}

func normalizedForwardedScheme(value string) string {
	scheme := strings.ToLower(strings.Trim(strings.TrimSpace(value), `"`))
	if scheme == "http" || scheme == "https" {
		return scheme
	}
	return ""
}
