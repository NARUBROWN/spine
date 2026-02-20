package ws

import (
	"context"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/event/publish"
	pkgws "github.com/NARUBROWN/spine/pkg/ws"
)

type connSender struct {
	send func(messageType int, data []byte) error
}

func (s *connSender) Send(messageType int, data []byte) error {
	return s.send(messageType, data)
}

type WSExecutionContext struct {
	ctx         context.Context
	connID      string
	path        string
	messageType int
	payload     []byte
	eventBus    publish.EventBus
	store       map[string]any
}

func NewWSExecutionContext(ctx context.Context, connID string, path string, messageType int, payload []byte, eventBus publish.EventBus, sendFn func(int, []byte) error) core.WebSocketContext {
	ctx = context.WithValue(ctx, pkgws.SenderKey, &connSender{send: sendFn})

	return &WSExecutionContext{
		ctx:         ctx,
		connID:      connID,
		path:        path,
		messageType: messageType,
		payload:     payload,
		eventBus:    eventBus,
		store:       make(map[string]any),
	}
}

func (w *WSExecutionContext) ConnID() string {
	return w.connID
}

func (w *WSExecutionContext) Context() context.Context {
	return w.ctx
}

func (w *WSExecutionContext) EventBus() core.EventBus {
	return w.eventBus
}

func (w *WSExecutionContext) Get(key string) (any, bool) {
	v, ok := w.store[key]
	return v, ok
}

func (w *WSExecutionContext) Header(name string) string {
	return ""
}

func (w *WSExecutionContext) MessageType() int {
	return w.messageType
}

func (w *WSExecutionContext) Method() string {
	return "WS"
}

func (w *WSExecutionContext) Params() map[string]string {
	return map[string]string{}
}

func (w *WSExecutionContext) Path() string {
	return w.path
}

func (w *WSExecutionContext) PathKeys() []string {
	return []string{}
}

func (w *WSExecutionContext) Payload() []byte {
	return w.payload
}

func (w *WSExecutionContext) Queries() map[string][]string {
	return map[string][]string{}
}

func (w *WSExecutionContext) Set(key string, value any) {
	w.store[key] = value
}
