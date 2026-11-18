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
	ctx := NewContext(context.Background(), Input{Routing: request, HTTPRequest: request, HTTPResponse: response})
	public := ctx.PublicView()
	httpView, _ := public.HTTPRequestContext()
	responseView, _ := public.HTTPResponseContext()
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 1000 {
				_ = httpView.Path()
				_ = responseView.IsCommitted()
			}
		}()
	}
	ctx.Close()
	ctx.Close()
	wg.Wait()
	if !reflect.DeepEqual(ctx.Input, Input{}) {
		t.Fatal("transport references remain")
	}
	if httpView.Path() != "" {
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
}

var retainedInputViews []contextview.Ctx

// Profile after completed executions while retaining their public contexts.
func BenchmarkCompletedInputLeaseChurn(b *testing.B) {
	retainedInputViews = make([]contextview.Ctx, 256)
	for n := 0; n < b.N; n++ {
		ctx := NewContext(context.Background(), Input{HTTPRequest: &requestOnlyContext{}})
		retainedInputViews[n%len(retainedInputViews)] = ctx.PublicView()
		ctx.Close()
	}
}
