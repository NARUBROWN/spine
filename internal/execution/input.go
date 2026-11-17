package execution

import "sync"

// ConsumerMessage excludes delivery settlement and native reader capabilities.
type ConsumerMessage struct {
	Name     string
	Payload  []byte
	Metadata map[string]string
}

// Sources are borrowed from the transport runtime for one pipeline execution.
// The runtime continues to own native connections, readers and ACK/NACK callbacks.
type Sources struct {
	Routing      RoutingContext
	HTTPRequest  HTTPRequestContext
	HTTPResponse HTTPResponseContext
	Consumer     *ConsumerMessage
}

// Input is a revocable view lease. Completion detaches every transport reference.
// Retaining a public Ctx or view cannot retain the received message after Close.
type Input struct {
	mu      sync.RWMutex
	sources Sources
	closed  bool
}

func NewInput(s Sources) *Input { return &Input{sources: s} }
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
func (i *Input) ConsumerMessage() (out ConsumerMessage, ok bool) {
	i.read(func(s Sources) {
		if s.Consumer != nil {
			out = *s.Consumer
			ok = true
		}
	})
	return
}
