package pipeline

import (
	"context"
	"github.com/NARUBROWN/spine/internal/execution"
)

func httpTestContext(source interface{ Context() context.Context }) *execution.Context {
	if ctx, ok := source.(*execution.Context); ok {
		return ctx
	}
	s := execution.Input{}
	s.HTTPRequest, _ = source.(execution.HTTPRequestContext)
	s.HTTPResponse, _ = source.(execution.HTTPResponseContext)
	s.Routing, _ = source.(execution.RoutingContext)
	return execution.NewContext(source.Context(), s)
}
