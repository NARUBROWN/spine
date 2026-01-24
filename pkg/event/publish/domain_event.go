package publish

import "time"

type DomainEvent interface {
	Name() string
	OccurredAt() time.Time
}
