package ws

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/NARUBROWN/spine/internal/event/publish"
	"github.com/NARUBROWN/spine/internal/pipeline"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Runtime struct {
	registry *Registry
	pipeline *pipeline.Pipeline
	stopOnce sync.Once
	ctx      context.Context
	cancel   context.CancelFunc
	connMu   sync.Mutex
	conns    map[string]*websocket.Conn
}

func NewRuntime(registry *Registry, pipeline *pipeline.Pipeline) *Runtime {
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
		ctx:      ctx,
		cancel:   cancel,
		conns:    make(map[string]*websocket.Conn),
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

	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		log.Printf("[WS] 업그레이드 실패 (%s): %v", reg.Path, err)
		return
	}

	connID := generateConnID()
	if !r.trackConn(connID, conn) {
		_ = conn.Close()
		return
	}
	defer func() {
		r.untrackConn(connID)
		_ = conn.Close()
	}()

	log.Printf("[WS] 연결 수립 (conn=%p, path=%s)", &connID, reg.Path)

	// 연결당 루프
	for {
		msgType, payload, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[WS] 연결 종료 (conn=%p): %v", &connID, err)
			return
		}

		eventBus := publish.NewEventBus()

		ctx := NewWSExecutionContext(
			req.Context(),
			connID,
			req.URL.Path,
			msgType,
			payload,
			eventBus,
			conn.WriteMessage,
		)

		if err := r.pipeline.Execute(ctx); err != nil {
			log.Printf("[WS] 핸들러 실패 (conn=%p): %v", &connID, err)
			_ = conn.WriteControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "handler error"),
				time.Now().Add(time.Second),
			)
			return
		}
	}
}

func (r *Runtime) Stop() {
	r.stopOnce.Do(func() {
		r.cancel()

		r.connMu.Lock()
		conns := make(map[string]*websocket.Conn, len(r.conns))
		for id, conn := range r.conns {
			conns[id] = conn
		}
		r.connMu.Unlock()

		for _, conn := range conns {
			_ = conn.WriteControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, "server shutting down"),
				time.Now().Add(time.Second),
			)
			_ = conn.Close()
		}

		log.Printf("[WS] WebSocket 런타임을 중지했습니다.")
	})
}

func (r *Runtime) trackConn(connID string, conn *websocket.Conn) bool {
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
