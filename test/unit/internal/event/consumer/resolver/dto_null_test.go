package resolver

import (
	"errors"
	"reflect"
	"testing"

	internalresolver "github.com/NARUBROWN/spine/internal/resolver"
	"github.com/NARUBROWN/spine/pkg/jsonx"
)

func TestDTOResolverRejectsScalarNullAndPreservesPointerNull(t *testing.T) {
	type payload struct {
		Count int  `json:"count"`
		Maybe *int `json:"maybe"`
	}
	r := &DTOResolver{}
	meta := internalresolver.ParameterMeta{Type: reflect.TypeFor[payload]()}
	_, err := r.Resolve(newTestConsumerContext("input", []byte(`{"count":null}`)), meta)
	var null *jsonx.NullError
	if !errors.As(err, &null) {
		t.Fatalf("소비자에서도 일반 값의 null을 거부해야 합니다: %v", err)
	}
	value, err := r.Resolve(newTestConsumerContext("input", []byte(`{"count":0,"maybe":null}`)), meta)
	if err != nil || value.(payload).Count != 0 || value.(payload).Maybe != nil {
		t.Fatalf("실제 0과 포인터의 null을 보존해야 합니다: %+v %v", value, err)
	}
}
