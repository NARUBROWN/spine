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
	msg      Message
	eventBus publish.EventBus
}

func NewRequestContext(
	ctx context.Context,
	msg Message,
	eventBus publish.EventBus,
) core.RequestContext {
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
	return c.eventBus
}

func (c *ConsumerRequestContextImpl) Bind(out any) error {
	return errors.New("ConsumerRequestContext에서는 Bind를 지원하지 않습니다")
}

func (c *ConsumerRequestContextImpl) MultipartForm() (*multipart.Form, error) {
	return nil, errors.New("ConsumerRequestContext에서는 Multipart를 지원하지 않습니다")
}
