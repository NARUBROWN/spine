package consumer

import (
	"context"
	"maps"
	"slices"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/event/publish"
)

type ConsumerRequestContextImpl struct {
	ctx      context.Context
	msg      *Message
	eventBus publish.EventBus
	store    map[string]any
	params   map[string]string
	pathKeys []string
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
	if c.eventBus == nil {
		c.eventBus = publish.NewEventBus()
	}
	return c.eventBus
}

func (c *ConsumerRequestContextImpl) Get(key string) (any, bool) {
	if c.store == nil {
		return nil, false
	}
	v, ok := c.store[key]
	return v, ok
}

func (c *ConsumerRequestContextImpl) Set(key string, value any) {
	if c.store == nil {
		c.store = make(map[string]any)
	}
	c.store[key] = value
}

func (c *ConsumerRequestContextImpl) Header(key string) string {
	// 컨슈머 실행 컨텍스트에는 HTTP 헤더 개념이 없으므로 항상 빈 문자열을 반환합니다.
	return ""
}

func (c *ConsumerRequestContextImpl) Method() string {
	// 컨슈머 실행에는 HTTP 메서드 개념이 없으며, 라우팅 구분을 위해 EVENT를 사용합니다.
	return "EVENT"
}

func (c *ConsumerRequestContextImpl) Path() string {
	// 컨슈머 라우팅의 경로에는 EventName을 그대로 사용합니다.
	return c.msg.EventName
}

func (c *ConsumerRequestContextImpl) Params() map[string]string { return maps.Clone(c.params) }

func (c *ConsumerRequestContextImpl) SetRoute(params map[string]string, pathKeys []string) {
	c.params = maps.Clone(params)
	c.pathKeys = slices.Clone(pathKeys)
}

func (c *ConsumerRequestContextImpl) PathKeys() []string { return slices.Clone(c.pathKeys) }

func (c *ConsumerRequestContextImpl) Queries() map[string][]string {
	return nil
}
