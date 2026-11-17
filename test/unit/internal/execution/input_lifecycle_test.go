package execution

import (
	"context"
	"github.com/NARUBROWN/spine/internal/contextview"
	"reflect"
	"sync"
	"testing"
)

func TestInputCloseDetachesAllSourcesAndRetainedViews(t *testing.T) {
	request := &requestOnlyContext{}
	response := &responseOnlyContext{writer: &viewWriter{headers: map[string]string{}}}
	input := NewInput(Sources{Routing: request, HTTPRequest: request, HTTPResponse: response, WebSocketRequest: leaseHandshake{}, ConnectionID: "connection", WebSocket: &WebSocketMessage{Kind: 1, Payload: []byte("original")}, Consumer: &ConsumerMessage{Name: "topic", Payload: []byte("event"), Metadata: map[string]string{"key": "value"}}})
	public := NewContext(NewState(context.Background()), input)
	httpView, _ := public.HTTPRequestContext()
	responseView, _ := public.HTTPResponseContext()
	wsView, _ := public.WebSocket()
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 1000 {
				_ = httpView.Path()
				_ = responseView.IsCommitted()
				_ = wsView.Payload()
				_ = wsView.ConnID()
				_ = wsView.Query("token")
			}
		}()
	}
	input.Close()
	input.Close()
	wg.Wait()
	if !reflect.DeepEqual(input.sources, Sources{}) {
		t.Fatal("transport references remain")
	}
	if httpView.Path() != "" || wsView.Payload() != nil || wsView.ConnID() != "" {
		t.Fatal("retained views still expose input")
	}
	if err := responseView.WriteString(200, "late"); err == nil {
		t.Fatal("completed response is writable")
	}
	if _, ok := public.HTTPRequestContext(); ok {
		t.Fatal("request still available")
	}
	if _, ok := public.HTTPResponseContext(); ok {
		t.Fatal("response still available")
	}
	if _, ok := public.WebSocket(); ok {
		t.Fatal("session still available")
	}
	if _, ok := input.ConsumerMessage(); ok {
		t.Fatal("event still available")
	}
}

type leaseHandshake struct{ WebSocketRequestContext }

func (leaseHandshake) Path() string        { return "/ws" }
func (leaseHandshake) Query(string) string { return "token" }

var retainedInputViews []contextview.Ctx

// Profile after completed executions while retaining their public contexts.
func BenchmarkCompletedInputLeaseChurn(b *testing.B) {
	retainedInputViews = make([]contextview.Ctx, 256)
	for n := 0; n < b.N; n++ {
		state := NewState(context.Background())
		input := NewInput(Sources{WebSocketRequest: leaseHandshake{}, ConnectionID: "connection", WebSocket: &WebSocketMessage{Kind: 1, Payload: make([]byte, 16<<10)}})
		retainedInputViews[n%len(retainedInputViews)] = NewContext(state, input)
		input.Close()
		state.ReleaseStore()
	}
}
