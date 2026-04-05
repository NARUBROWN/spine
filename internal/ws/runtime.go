package ws

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/NARUBROWN/spine/internal/pipeline"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/gorilla/websocket"
)

const (
	defaultWSHandshakeTimeout = 10 * time.Second
	defaultWSReadTimeout      = 60 * time.Second
	defaultWSWriteTimeout     = 10 * time.Second
	defaultWSMaxMessageBytes  = 1 << 20
)

type normalizedWebSocketOptions struct {
	AllowedOrigins   []string
	MaxMessageBytes  int64
	HandshakeTimeout time.Duration
	ReadTimeout      time.Duration
	WriteTimeout     time.Duration
	PingInterval     time.Duration
}

type Runtime struct {
	registry *Registry
	pipeline *pipeline.Pipeline
	options  normalizedWebSocketOptions
	stopOnce sync.Once
	ctx      context.Context
	cancel   context.CancelFunc
	connMu   sync.Mutex
	conns    map[string]*trackedConn
}

type trackedConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *trackedConn) writeMessage(messageType int, data []byte, timeout time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if timeout > 0 {
		_ = c.conn.SetWriteDeadline(time.Now().Add(timeout))
	}
	return c.conn.WriteMessage(messageType, data)
}

func (c *trackedConn) writeControl(messageType int, data []byte, timeout time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	deadline := time.Time{}
	if timeout > 0 {
		deadline = time.Now().Add(timeout)
		_ = c.conn.SetWriteDeadline(deadline)
	}
	return c.conn.WriteControl(messageType, data, deadline)
}

func NewRuntime(registry *Registry, pipeline *pipeline.Pipeline, opts boot.WebSocketOptions) *Runtime {
	if registry == nil {
		panic("ws: registry는 nil일 수 없습니다")
	}
	if pipeline == nil {
		panic("ws: pipeline은 nil일 수 없습니다")
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Runtime{
		registry: registry,
		pipeline: pipeline,
		options:  normalizeWebSocketOptions(opts),
		ctx:      ctx,
		cancel:   cancel,
		conns:    make(map[string]*trackedConn),
	}
}

func normalizeWebSocketOptions(opts boot.WebSocketOptions) normalizedWebSocketOptions {
	normalized := normalizedWebSocketOptions{
		AllowedOrigins:   append([]string(nil), opts.AllowedOrigins...),
		MaxMessageBytes:  opts.MaxMessageBytes,
		HandshakeTimeout: opts.HandshakeTimeout,
		ReadTimeout:      opts.ReadTimeout,
		WriteTimeout:     opts.WriteTimeout,
		PingInterval:     opts.PingInterval,
	}

	if normalized.MaxMessageBytes == 0 {
		normalized.MaxMessageBytes = defaultWSMaxMessageBytes
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

func (r *Runtime) upgrader() websocket.Upgrader {
	return websocket.Upgrader{
		HandshakeTimeout: r.options.HandshakeTimeout,
		CheckOrigin: func(req *http.Request) bool {
			return isAllowedWebSocketOrigin(req, r.options.AllowedOrigins)
		},
	}
}

// Mount는 각 WebSocket 경로를 http.ServeMux에 등록합니다.
func (r *Runtime) Mount(mux *http.ServeMux) {
	for _, reg := range r.registry.Registrations() {
		reg := reg
		log.Printf("[WS] 경로 등록: %s", reg.Path)

		mux.HandleFunc(reg.Path, func(w http.ResponseWriter, req *http.Request) {
			r.HandleConn(w, req, reg)
		})
	}
}

func (r *Runtime) HandleConn(w http.ResponseWriter, req *http.Request, reg Registration) {
	select {
	case <-r.ctx.Done():
		http.Error(w, "websocket runtime is shutting down", http.StatusServiceUnavailable)
		return
	default:
	}

	upgrader := r.upgrader()
	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		log.Printf("[WS] 업그레이드 실패 (%s): %v", reg.Path, err)
		return
	}

	connID := generateConnID()
	tracked := &trackedConn{conn: conn}
	if !r.trackConn(connID, tracked) {
		_ = conn.Close()
		return
	}
	defer func() {
		r.untrackConn(connID)
		_ = conn.Close()
	}()

	log.Printf("[WS] 연결 수립 (conn=%p, path=%s)", &connID, reg.Path)

	if r.options.MaxMessageBytes > 0 {
		conn.SetReadLimit(r.options.MaxMessageBytes)
	}
	_ = conn.SetReadDeadline(time.Now().Add(r.options.ReadTimeout))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(r.options.ReadTimeout))
	})

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
			case <-r.ctx.Done():
				return
			case <-req.Context().Done():
				return
			case <-ticker.C:
				err := tracked.writeControl(websocket.PingMessage, nil, r.options.WriteTimeout)
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
			log.Printf("[WS] 연결 종료 (conn=%p): %v", &connID, err)
			return
		}

		ctx := NewWSExecutionContext(
			req.Context(),
			connID,
			req.URL.Path,
			msgType,
			payload,
			nil,
			sendFn,
		)

		if err := r.pipeline.Execute(ctx); err != nil {
			log.Printf("[WS] 핸들러 실패 (conn=%p): %v", &connID, err)
			_ = tracked.writeControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "handler error"),
				r.options.WriteTimeout,
			)
			return
		}
		_ = conn.SetReadDeadline(time.Now().Add(r.options.ReadTimeout))
	}
}

func (r *Runtime) Stop() {
	r.stopOnce.Do(func() {
		r.cancel()

		r.connMu.Lock()
		conns := make(map[string]*trackedConn, len(r.conns))
		for id, conn := range r.conns {
			conns[id] = conn
		}
		r.connMu.Unlock()

		for _, conn := range conns {
			_ = conn.writeControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, "server shutting down"),
				r.options.WriteTimeout,
			)
			_ = conn.conn.Close()
		}

		log.Printf("[WS] WebSocket 런타임을 중지했습니다.")
	})
}

func (r *Runtime) trackConn(connID string, conn *trackedConn) bool {
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

func (r *Runtime) untrackConn(connID string) {
	r.connMu.Lock()
	defer r.connMu.Unlock()
	delete(r.conns, connID)
}

func isAllowedWebSocketOrigin(req *http.Request, allowedOrigins []string) bool {
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

	return originURL.Scheme == "http" || originURL.Scheme == "https"
}
