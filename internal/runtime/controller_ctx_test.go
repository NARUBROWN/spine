package runtime

import (
	"context"
	"testing"

	"github.com/NARUBROWN/spine/core"
)

type testExecutionContext struct {
	store map[string]any
}

func (c *testExecutionContext) Context() context.Context     { return context.Background() }
func (c *testExecutionContext) EventBus() core.EventBus      { return nil }
func (c *testExecutionContext) Method() string               { return "GET" }
func (c *testExecutionContext) Path() string                 { return "/" }
func (c *testExecutionContext) Params() map[string]string    { return map[string]string{} }
func (c *testExecutionContext) Header(name string) string    { return "" }
func (c *testExecutionContext) PathKeys() []string           { return nil }
func (c *testExecutionContext) Queries() map[string][]string { return map[string][]string{} }
func (c *testExecutionContext) Set(key string, value any)    { c.store[key] = value }
func (c *testExecutionContext) Get(key string) (any, bool)   { v, ok := c.store[key]; return v, ok }

func TestNewControllerContext_ReadsFromExecutionContext(t *testing.T) {
	ec := &testExecutionContext{store: map[string]any{"userID": 7}}
	ctx := NewControllerContext(ec)

	got, ok := ctx.Get("userID")
	if !ok || got.(int) != 7 {
		t.Fatalf("controller context는 execution context를 그대로 읽어야 합니다: %v %v", got, ok)
	}
}
