package resolver

import (
	"context"
	"reflect"
	"testing"

	"github.com/NARUBROWN/spine/core"
	internalpublish "github.com/NARUBROWN/spine/internal/event/publish"
	internalresolver "github.com/NARUBROWN/spine/internal/resolver"
)

type testConsumerContext struct {
	eventName string
	payload   []byte
	store     map[string]any
}

func newTestConsumerContext(eventName string, payload []byte) *testConsumerContext {
	return &testConsumerContext{
		eventName: eventName,
		payload:   payload,
		store:     map[string]any{},
	}
}

func (c *testConsumerContext) Context() context.Context     { return context.Background() }
func (c *testConsumerContext) EventBus() core.EventBus      { return internalpublish.NewEventBus() }
func (c *testConsumerContext) Method() string               { return "EVENT" }
func (c *testConsumerContext) Path() string                 { return c.eventName }
func (c *testConsumerContext) Params() map[string]string    { return map[string]string{} }
func (c *testConsumerContext) Header(name string) string    { return "" }
func (c *testConsumerContext) PathKeys() []string           { return nil }
func (c *testConsumerContext) Queries() map[string][]string { return map[string][]string{} }
func (c *testConsumerContext) Set(key string, value any)    { c.store[key] = value }
func (c *testConsumerContext) Get(key string) (any, bool)   { v, ok := c.store[key]; return v, ok }
func (c *testConsumerContext) EventName() string            { return c.eventName }
func (c *testConsumerContext) Payload() []byte              { return c.payload }

type nonConsumerContext struct{}

func (c *nonConsumerContext) Context() context.Context     { return context.Background() }
func (c *nonConsumerContext) EventBus() core.EventBus      { return internalpublish.NewEventBus() }
func (c *nonConsumerContext) Method() string               { return "GET" }
func (c *nonConsumerContext) Path() string                 { return "/" }
func (c *nonConsumerContext) Params() map[string]string    { return map[string]string{} }
func (c *nonConsumerContext) Header(name string) string    { return "" }
func (c *nonConsumerContext) PathKeys() []string           { return nil }
func (c *nonConsumerContext) Queries() map[string][]string { return map[string][]string{} }
func (c *nonConsumerContext) Set(key string, value any)    {}
func (c *nonConsumerContext) Get(key string) (any, bool)   { return nil, false }

func TestEventNameResolver(t *testing.T) {
	r := &EventNameResolver{}
	pm := internalresolver.ParameterMeta{Type: reflect.TypeOf("")}

	if !r.Supports(pm) {
		t.Fatal("string은 EventNameResolver가 지원해야 합니다")
	}

	val, err := r.Resolve(newTestConsumerContext("order.created", nil), pm)
	if err != nil {
		t.Fatalf("Resolve 실패: %v", err)
	}
	if val.(string) != "order.created" {
		t.Fatalf("event name이 잘못되었습니다: %v", val)
	}

	if _, err := r.Resolve(newTestConsumerContext("", nil), pm); err == nil {
		t.Fatal("빈 event name은 에러여야 합니다")
	}
	if _, err := r.Resolve(&nonConsumerContext{}, pm); err == nil {
		t.Fatal("ConsumerRequestContext가 아니면 에러여야 합니다")
	}
}

func TestPayloadResolver(t *testing.T) {
	r := &PayloadResolver{}
	pm := internalresolver.ParameterMeta{Type: reflect.TypeOf([]byte{})}

	if !r.Supports(pm) {
		t.Fatal("[]byte는 PayloadResolver가 지원해야 합니다")
	}

	val, err := r.Resolve(newTestConsumerContext("order.created", []byte("hello")), pm)
	if err != nil {
		t.Fatalf("Resolve 실패: %v", err)
	}
	if string(val.([]byte)) != "hello" {
		t.Fatalf("payload가 잘못되었습니다: %s", string(val.([]byte)))
	}

	if _, err := r.Resolve(newTestConsumerContext("order.created", nil), pm); err == nil {
		t.Fatal("nil payload는 에러여야 합니다")
	}
	if _, err := r.Resolve(&nonConsumerContext{}, pm); err == nil {
		t.Fatal("ConsumerRequestContext가 아니면 에러여야 합니다")
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
		t.Fatal("struct는 DTOResolver가 지원해야 합니다")
	}

	val, err := r.Resolve(newTestConsumerContext("order.created", []byte(`{"name":"kim","age":20}`)), pm)
	if err != nil {
		t.Fatalf("Resolve 실패: %v", err)
	}
	dto := val.(sampleDTO)
	if dto.Name != "kim" || dto.Age != 20 {
		t.Fatalf("DTO 값이 잘못되었습니다: %+v", dto)
	}

	if _, err := r.Resolve(newTestConsumerContext("order.created", nil), pm); err == nil {
		t.Fatal("nil payload는 에러여야 합니다")
	}
	if _, err := r.Resolve(newTestConsumerContext("order.created", []byte(`{"name"`)), pm); err == nil {
		t.Fatal("잘못된 JSON은 에러여야 합니다")
	}
	if _, err := r.Resolve(&nonConsumerContext{}, pm); err == nil {
		t.Fatal("ConsumerRequestContext가 아니면 에러여야 합니다")
	}
}
