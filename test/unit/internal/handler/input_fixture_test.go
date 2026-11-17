package handler

import "github.com/NARUBROWN/spine/internal/execution"

func httpTestInput(ctx execution.Context) *execution.Input {
	s := execution.Sources{}
	s.HTTPRequest, _ = ctx.(execution.HTTPRequestContext)
	s.HTTPResponse, _ = ctx.(execution.HTTPResponseContext)
	s.Routing, _ = ctx.(execution.RoutingContext)
	return execution.NewInput(s)
}
