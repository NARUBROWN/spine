package publish

import (
	"context"

	"github.com/NARUBROWN/spine/pkg/event/publish"
)

type EventPublisher interface {
	Publish(ctx context.Context, event publish.DomainEvent) error
}
