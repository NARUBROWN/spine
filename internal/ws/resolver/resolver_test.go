package resolver

import (
	"context"
	"reflect"
	"testing"

	"github.com/NARUBROWN/spine/core"
	internalpublish "github.com/NARUBROWN/spine/internal/event/publish"
	internalresolver "github.com/NARUBROWN/spine/internal/resolver"
	pkgws "github.com/NARUBROWN/spine/pkg/ws"
)

type testWSContext struct {
	connID      string
	messageType int
	payload     []byte
	store       map[string]any
}

func newTestWSContext(payload []byte) *testWSContext {
	return &testWSContext{
		connID:      "abc123",
		messageType: pkgws.TextMessage,
		payload:     payload,
		store:       map[string]any{},
	}
}

func (c *testWSContext) Context() context.Context     { return context.Background() }
func (c *testWSContext) EventBus() core.EventBus      { return internalpublish.NewEventBus() }
func (c *testWSContext) Method() string               { return "WS" }
func (c *testWSContext) Path() string                 { return c.connID }
func (c *testWSContext) Params() map[string]string    { return map[string]string{} }
func (c *testWSContext) Header(name string) string    { return "" }
func (c *testWSContext) PathKeys() []string           { return []string{} }
func (c *testWSContext) Queries() map[string][]string { return map[string][]string{} }
func (c *testWSContext) Set(key string, value any)    { c.store[key] = value }
func (c *testWSContext) Get(key string) (any, bool)   { v, ok := c.store[key]; return v, ok }
func (c *testWSContext) ConnID() string               { return c.connID }
func (c *testWSContext) MessageType() int             { return c.messageType }
func (c *testWSContext) Payload() []byte              { return c.payload }

type nonWSContext struct{}

func (c *nonWSContext) Context() context.Context     { return context.Background() }
func (c *nonWSContext) EventBus() core.EventBus      { return internalpublish.NewEventBus() }
func (c *nonWSContext) Method() string               { return "GET" }
func (c *nonWSContext) Path() string                 { return "/" }
func (c *nonWSContext) Params() map[string]string    { return map[string]string{} }
func (c *nonWSContext) Header(name string) string    { return "" }
func (c *nonWSContext) PathKeys() []string           { return []string{} }
func (c *nonWSContext) Queries() map[string][]string { return map[string][]string{} }
func (c *nonWSContext) Set(key string, value any)    {}
func (c *nonWSContext) Get(key string) (any, bool)   { return nil, false }

func TestConnectionIDResolver(t *testing.T) {
	r := &ConnectionIDResolver{}
	pm := internalresolver.ParameterMeta{Type: reflect.TypeFor[pkgws.ConnectionID]()}

	if !r.Supports(pm) {
		t.Fatal("ConnectionIDResolver가 ConnectionID를 지원해야 합니다")
	}

	val, err := r.Resolve(newTestWSContext(nil), pm)
	if err != nil {
		t.Fatalf("ConnectionIDResolver 실패: %v", err)
	}
	if val.(pkgws.ConnectionID).Value != "abc123" {
		t.Fatalf("ConnectionID 값이 잘못되었습니다: %v", val)
	}

	_, err = r.Resolve(&nonWSContext{}, pm)
	if err == nil {
		t.Fatal("WebSocketContext가 아니면 에러여야 합니다")
	}
}

func TestPayloadResolver(t *testing.T) {
	r := &PayloadResolver{}
	pm := internalresolver.ParameterMeta{Type: reflect.TypeOf([]byte{})}

	if !r.Supports(pm) {
		t.Fatal("PayloadResolver가 []byte를 지원해야 합니다")
	}

	val, err := r.Resolve(newTestWSContext([]byte("hello")), pm)
	if err != nil {
		t.Fatalf("PayloadResolver 실패: %v", err)
	}
	if string(val.([]byte)) != "hello" {
		t.Fatalf("payload 값이 잘못되었습니다: %s", string(val.([]byte)))
	}
}

func TestDTOResolver(t *testing.T) {
	type sampleDTO struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	r := &DTOResolver{}
	pm := internalresolver.ParameterMeta{Type: reflect.TypeFor[sampleDTO]()}

	if !r.Supports(pm) {
		t.Fatal("DTOResolver가 struct DTO를 지원해야 합니다")
	}

	val, err := r.Resolve(newTestWSContext([]byte(`{"name":"kim","age":20}`)), pm)
	if err != nil {
		t.Fatalf("DTOResolver 실패: %v", err)
	}
	dto := val.(sampleDTO)
	if dto.Name != "kim" || dto.Age != 20 {
		t.Fatalf("DTO 변환 결과가 잘못되었습니다: %+v", dto)
	}

	_, err = r.Resolve(newTestWSContext(nil), pm)
	if err == nil {
		t.Fatal("payload가 nil이면 에러여야 합니다")
	}

	_, err = r.Resolve(newTestWSContext([]byte(`{"name"`)), pm)
	if err == nil {
		t.Fatal("잘못된 JSON이면 에러여야 합니다")
	}
}
