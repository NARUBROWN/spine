package consumer

import "context"

type Reader interface {
	Read(ctx context.Context) (Message, error)
	Close() error
}
