package execution

func httpTestInput(ctx Context) *Input {
	s := Sources{}
	s.HTTPRequest, _ = ctx.(HTTPRequestContext)
	s.HTTPResponse, _ = ctx.(HTTPResponseContext)
	s.Routing, _ = ctx.(RoutingContext)
	return NewInput(s)
}
