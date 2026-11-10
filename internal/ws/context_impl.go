package ws

import (
	"context"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"sync"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/event/emission"
	"github.com/NARUBROWN/spine/internal/event/publish"
	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/ws/sendctx"
)

type WSExecutionContext struct {
	mu          sync.RWMutex
	ctx         context.Context
	connID      string
	path        string
	messageType int
	payload     []byte
	eventBus    publish.EventBus
	params      map[string]string
	pathKeys    []string
	store       map[string]any
	request     *webSocketRequestSnapshot
}

type webSocketRequestSnapshot struct {
	ctx        context.Context
	path       string
	headers    http.Header
	queries    url.Values
	cookies    map[string]string
	remoteAddr string
	host       string
	requestURI string
}

func newWebSocketRequestSnapshot(req *http.Request) *webSocketRequestSnapshot {
	if req == nil {
		return &webSocketRequestSnapshot{
			ctx:     context.Background(),
			headers: make(http.Header),
			queries: make(url.Values),
			cookies: make(map[string]string),
		}
	}
	queries := make(url.Values, len(req.URL.Query()))
	for key, values := range req.URL.Query() {
		queries[key] = append([]string(nil), values...)
	}
	cookies := make(map[string]string)
	for _, cookie := range req.Cookies() {
		if _, exists := cookies[cookie.Name]; !exists {
			cookies[cookie.Name] = cookie.Value
		}
	}
	return &webSocketRequestSnapshot{
		ctx:        req.Context(),
		path:       req.URL.Path,
		headers:    req.Header.Clone(),
		queries:    queries,
		cookies:    cookies,
		remoteAddr: req.RemoteAddr,
		host:       req.Host,
		requestURI: req.RequestURI,
	}
}

func (s *webSocketRequestSnapshot) Context() context.Context  { return s.ctx }
func (s *webSocketRequestSnapshot) Path() string              { return s.path }
func (s *webSocketRequestSnapshot) Header(name string) string { return s.headers.Get(name) }
func (s *webSocketRequestSnapshot) Headers() map[string][]string {
	return s.headers.Clone()
}
func (s *webSocketRequestSnapshot) Query(name string) string { return s.queries.Get(name) }
func (s *webSocketRequestSnapshot) Queries() map[string][]string {
	copyValues := make(map[string][]string, len(s.queries))
	for key, values := range s.queries {
		copyValues[key] = append([]string(nil), values...)
	}
	return copyValues
}
func (s *webSocketRequestSnapshot) Cookie(name string) (string, bool) {
	value, ok := s.cookies[name]
	return value, ok
}
func (s *webSocketRequestSnapshot) Cookies() map[string]string {
	copyCookies := make(map[string]string, len(s.cookies))
	for name, value := range s.cookies {
		copyCookies[name] = value
	}
	return copyCookies
}
func (s *webSocketRequestSnapshot) RemoteAddr() string { return s.remoteAddr }
func (s *webSocketRequestSnapshot) Host() string       { return s.host }
func (s *webSocketRequestSnapshot) RequestURI() string { return s.requestURI }

func NewWSExecutionContext(ctx context.Context, connID string, path string, messageType int, payload []byte, eventBus publish.EventBus, sendFn func(int, []byte) error, request ...core.WebSocketHandshakeContext) execution.WebSocketContext {
	ctx = sendctx.With(ctx, sendFn)
	requestSnapshot := &webSocketRequestSnapshot{
		ctx:     ctx,
		path:    path,
		headers: make(http.Header),
		queries: make(url.Values),
		cookies: make(map[string]string),
	}
	if len(request) > 0 && request[0] != nil {
		requestSnapshot = snapshotWebSocketRequest(ctx, request[0])
	}

	return &WSExecutionContext{
		ctx:         ctx,
		connID:      connID,
		path:        path,
		messageType: messageType,
		payload:     payload,
		eventBus:    eventBus,
		request:     requestSnapshot,
	}
}

func snapshotWebSocketRequest(ctx context.Context, request core.WebSocketHandshakeContext) *webSocketRequestSnapshot {
	headers := make(http.Header)
	for key, values := range request.Headers() {
		headers[key] = append([]string(nil), values...)
	}
	queries := make(url.Values)
	for key, values := range request.Queries() {
		queries[key] = append([]string(nil), values...)
	}
	cookies := make(map[string]string)
	for name, value := range request.Cookies() {
		cookies[name] = value
	}
	return &webSocketRequestSnapshot{
		ctx:        ctx,
		path:       request.Path(),
		headers:    headers,
		queries:    queries,
		cookies:    cookies,
		remoteAddr: request.RemoteAddr(),
		host:       request.Host(),
		requestURI: request.RequestURI(),
	}
}

func (w *WSExecutionContext) ConnID() string {
	return w.connID
}

func (w *WSExecutionContext) Context() context.Context {
	return w.ctx
}

func (w *WSExecutionContext) EventBus() emission.Bus {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.eventBus == nil {
		w.eventBus = publish.NewEventBus()
	}
	return w.eventBus
}

func (w *WSExecutionContext) Get(key string) (any, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.store == nil {
		return nil, false
	}
	v, ok := w.store[key]
	return v, ok
}

func (w *WSExecutionContext) Header(name string) string {
	return w.request.Header(name)
}

func (w *WSExecutionContext) Headers() map[string][]string { return w.request.Headers() }
func (w *WSExecutionContext) Query(name string) string     { return w.request.Query(name) }
func (w *WSExecutionContext) Cookie(name string) (string, bool) {
	return w.request.Cookie(name)
}
func (w *WSExecutionContext) Cookies() map[string]string { return w.request.Cookies() }
func (w *WSExecutionContext) RemoteAddr() string         { return w.request.RemoteAddr() }
func (w *WSExecutionContext) Host() string               { return w.request.Host() }
func (w *WSExecutionContext) RequestURI() string         { return w.request.RequestURI() }

func (w *WSExecutionContext) MessageType() int {
	return w.messageType
}

func (w *WSExecutionContext) Method() string {
	return "WS"
}

func (w *WSExecutionContext) Params() map[string]string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.params == nil {
		return map[string]string{}
	}
	return maps.Clone(w.params)
}

func (w *WSExecutionContext) SetRoute(params map[string]string, pathKeys []string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.params = maps.Clone(params)
	w.pathKeys = slices.Clone(pathKeys)
}

func (w *WSExecutionContext) Path() string {
	return w.path
}

func (w *WSExecutionContext) PathKeys() []string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.pathKeys == nil {
		return []string{}
	}
	return slices.Clone(w.pathKeys)
}

func (w *WSExecutionContext) Payload() []byte {
	return w.payload
}

func (w *WSExecutionContext) Queries() map[string][]string {
	return w.request.Queries()
}

func (w *WSExecutionContext) Set(key string, value any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.store == nil {
		w.store = make(map[string]any)
	}
	w.store[key] = value
}
