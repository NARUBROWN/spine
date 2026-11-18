package execution

import (
	"context"
)

func httpTestContext(source interface{ Context() context.Context }) *Context {
	if ctx, ok := source.(*Context); ok {
		return ctx
	}
	s := Input{}
	s.HTTPRequest, _ = source.(HTTPRequestContext)
	s.HTTPResponse, _ = source.(HTTPResponseContext)
	s.Routing, _ = source.(RoutingContext)
	return NewContext(source.Context(), s)
}
