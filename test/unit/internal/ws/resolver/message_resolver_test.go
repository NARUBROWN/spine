package resolver

import (
	"context"
	"github.com/NARUBROWN/spine/internal/resolver"
	"github.com/NARUBROWN/spine/pkg/input"
	"reflect"
	"testing"
)

type changingInput struct{ Value string }

func (v *changingInput) UnmarshalJSON(data []byte) error {
	data[0] = 'x'
	v.Value = "decoded"
	return nil
}
func TestMessageWrapperIsolatesRawInputAndCustomDecoder(t *testing.T) {
	r := &MessageResolver{}
	source := newTestWSContext([]byte(`{"text":"hello"}`))
	for _, typ := range []reflect.Type{reflect.TypeFor[input.Message[[]byte]](), reflect.TypeFor[input.Message[changingInput]]()} {
		got, err := r.Resolve(source, resolver.ParameterMeta{Type: typ}, testInput(source))
		if err != nil {
			t.Fatal(err)
		}
		if raw, ok := got.(input.Message[[]byte]); ok {
			raw.Value[0] = 'x'
		}
		if source.payload[0] != '{' {
			t.Fatal("decoder changed transport-owned bytes")
		}
	}
	if _, err := r.Resolve(source, resolver.ParameterMeta{Type: reflect.TypeFor[input.Message[int]]()}, testInput(source)); err == nil {
		t.Fatal("unsupported payload type accepted")
	}
}

type messageValidator struct{ calls int }

func (v *messageValidator) Validate(ctx context.Context, value any) error {
	v.calls++
	if _, ok := value.(changingInput); !ok {
		panic("wrapper passed to validator")
	}
	return nil
}
func TestMessageWrapperValidatesDecodedValue(t *testing.T) {
	v := &messageValidator{}
	r := &MessageResolver{Validator: v}
	if _, err := r.Resolve(newTestWSContext([]byte(`{}`)), resolver.ParameterMeta{Type: reflect.TypeFor[input.Message[changingInput]]()}, testInput(newTestWSContext([]byte(`{}`)))); err != nil || v.calls != 1 {
		t.Fatalf("calls=%d err=%v", v.calls, err)
	}
}
