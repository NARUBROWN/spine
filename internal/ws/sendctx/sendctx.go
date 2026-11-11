package sendctx

import "context"

type senderKey struct{}

func With(ctx context.Context, send func(int, []byte) error) context.Context {
	return context.WithValue(ctx, senderKey{}, send)
}

func From(ctx context.Context) func(int, []byte) error {
	send, _ := ctx.Value(senderKey{}).(func(int, []byte) error)
	return send
}
