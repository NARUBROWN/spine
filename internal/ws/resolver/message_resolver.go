package resolver

import (
	"context"
	"fmt"
	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/internal/jsondecode"
	"github.com/NARUBROWN/spine/internal/resolver"
	"reflect"
	"slices"
	"strings"
)

// MessagePayload recognizes the explicit WebSocket input wrapper.
func MessagePayload(t reflect.Type) (reflect.Type, bool) {
	if t == nil || t.Kind() != reflect.Struct || t.PkgPath() != "github.com/NARUBROWN/spine/pkg/input" || !strings.HasPrefix(t.Name(), "Message[") {
		return nil, false
	}
	field, ok := t.FieldByName("Value")
	return field.Type, ok
}

type MessageResolver struct{ Validator validation.Validator }

func (*MessageResolver) Supports(p resolver.ParameterMeta) bool {
	_, ok := MessagePayload(p.Type)
	return ok
}
func (r *MessageResolver) Resolve(ctx context.Context, p resolver.ParameterMeta, source *Input) (any, error) {
	message := source
	if message == nil {
		return nil, fmt.Errorf("input.Message requires a WebSocket message")
	}
	t, _ := MessagePayload(p.Type)
	result := reflect.New(p.Type).Elem()
	if t == reflect.TypeFor[[]byte]() {
		result.FieldByName("Value").SetBytes(slices.Clone(message.Payload))
		return result.Interface(), nil
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("input.Message requires a struct or []byte payload")
	}
	value := reflect.New(t)
	if err := jsondecode.Unmarshal(slices.Clone(message.Payload), value.Interface()); err != nil {
		return nil, fmt.Errorf("decode WebSocket message: %w", err)
	}
	if err := resolver.ValidateDTO(ctx, r.Validator, value.Elem().Interface()); err != nil {
		return nil, err
	}
	result.FieldByName("Value").Set(value.Elem())
	return result.Interface(), nil
}
