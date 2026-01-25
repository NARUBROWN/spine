package consumer

import (
	"context"
	"errors"
	"mime/multipart"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/event/publish"
)

type ConsumerRequestContextImpl struct {
	ctx      context.Context
	msg      *Message
	eventBus publish.EventBus
	store    map[string]any
}

func NewRequestContext(
	ctx context.Context,
	msg *Message,
	eventBus publish.EventBus,
) core.ExecutionContext {
	return &ConsumerRequestContextImpl{
		ctx:      ctx,
		msg:      msg,
		eventBus: eventBus,
		store:    make(map[string]any),
	}
}

func (c *ConsumerRequestContextImpl) Context() context.Context {
	return c.ctx
}

func (c *ConsumerRequestContextImpl) EventName() string {
	return c.msg.EventName
}

func (c *ConsumerRequestContextImpl) Payload() []byte {
	return c.msg.Payload
}

func (c *ConsumerRequestContextImpl) EventBus() publish.EventBus {
	return c.eventBus
}

func (c *ConsumerRequestContextImpl) Bind(out any) error {
	return errors.New("ConsumerRequestContext에서는 Bind를 지원하지 않습니다")
}

func (c *ConsumerRequestContextImpl) MultipartForm() (*multipart.Form, error) {
	return nil, errors.New("ConsumerRequestContext에서는 Multipart를 지원하지 않습니다")
}

func (c *ConsumerRequestContextImpl) Request() core.RequestContext {
	return c
}

func (c *ConsumerRequestContextImpl) Get(key string) (any, bool) {
	v, ok := c.store[key]
	return v, ok
}

func (c *ConsumerRequestContextImpl) Set(key string, value any) {
	c.store[key] = value
}

func (c *ConsumerRequestContextImpl) Header(key string) string {
	// Consumer 실행 컨텍스트에는 HTTP Header 개념이 없으므로 항상 빈 문자열을 반환합니다.
	return ""
}

func (c *ConsumerRequestContextImpl) Method() string {
	// Consumer 실행은 HTTP Method 개념이 없으며, 라우팅 구분을 위해 EVENT를 사용합니다.
	return "EVENT"
}

func (c *ConsumerRequestContextImpl) Path() string {
	// Consumer 라우팅에서 Path는 EventName을 그대로 사용합니다.
	return c.msg.EventName
}

func (c *ConsumerRequestContextImpl) Params() map[string]string {
	// Consumer 실행에는 Path Parameter 개념이 없습니다.
	return map[string]string{}
}

func (c *ConsumerRequestContextImpl) PathKeys() []string {
	// Consumer 실행에는 Path Key 개념이 없습니다.
	return []string{}
}

func (c *ConsumerRequestContextImpl) Queries() map[string][]string {
	// Consumer 실행에는 Query Parameter 개념이 없습니다.
	return map[string][]string{}
}
