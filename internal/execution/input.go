package execution

import "sync"

// Sources contains the HTTP interfaces borrowed for one pipeline execution.
type Sources struct {
	Routing      RoutingContext
	HTTPRequest  HTTPRequestContext
	HTTPResponse HTTPResponseContext
}

// Input is a revocable view lease. Completion detaches every transport reference.
// Close detaches the HTTP request and response from retained public views.
type Input struct {
	mu      sync.RWMutex
	sources Sources
	closed  bool
}

func (i *Input) Close() {
	if i == nil {
		return
	}
	i.mu.Lock()
	i.closed = true
	i.sources = Sources{}
	i.mu.Unlock()
}
func (i *Input) read(f func(Sources)) {
	if i == nil {
		return
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	if !i.closed {
		f(i.sources)
	}
}
func (i *Input) HTTPRequest() (out HTTPRequestContext) {
	i.read(func(s Sources) { out = s.HTTPRequest })
	return
}
func (i *Input) HTTPResponse() (out HTTPResponseContext) {
	i.read(func(s Sources) { out = s.HTTPResponse })
	return
}
func (i *Input) Routing() (out RoutingContext) { i.read(func(s Sources) { out = s.Routing }); return }
