package resolver

import (
	"context"
	"errors"
	"reflect"
	"testing"

	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/pkg/httperr"
)

func TestInputDTOResolverBindsOnlyTheDeclaredSourceBeforeValidation(t *testing.T) {
	type filters struct {
		Age  int  `query:"age"`
		Zero *int `query:"zero"`
	}
	type headers struct {
		ID    string   `header:"X-Request-ID"`
		Roles []string `header:"X-Roles"`
	}
	ctx := newFakeHttpCtx()
	ctx.bindErr = errors.New("본문 바인더를 호출하면 안 됩니다")
	ctx.queries = map[string][]string{"age": {"18"}, "zero": {"0"}, "X-Request-ID": {"query-spoof"}}
	ctx.headers = map[string][]string{"x-request-id": {"request-1"}, "X-Roles": {"reader, writer"}, "age": {"999"}}
	calls := 0
	r := &InputDTOResolver{Validator: validatorFunc(func(_ context.Context, value any) error { calls++; return nil })}
	query, err := r.Resolve(ctx, ParameterMeta{Type: reflect.TypeFor[*filters]()})
	if err != nil {
		t.Fatal(err)
	}
	q := query.(*filters)
	if q.Age != 18 || q.Zero == nil || *q.Zero != 0 {
		t.Fatalf("쿼리 구조체 직접 접근: %+v", q)
	}
	value, err := r.Resolve(ctx, ParameterMeta{Type: reflect.TypeFor[*headers]()})
	if err != nil {
		t.Fatal(err)
	}
	h := value.(*headers)
	if h.ID != "request-1" || len(h.Roles) != 2 || calls != 2 {
		t.Fatalf("헤더 바인딩과 검증: %+v %d", h, calls)
	}
	ctx.queries["age"] = []string{"secret-not-an-int"}
	_, err = r.Resolve(ctx, ParameterMeta{Type: reflect.TypeFor[*filters]()})
	var failure *httperr.HTTPError
	if !errors.As(err, &failure) || failure.Status != 400 || calls != 2 {
		t.Fatalf("변환 실패는 검증 전 400이어야 합니다: %v %d", err, calls)
	}
}

func TestInputDTOResolverPreservesValidationFailure(t *testing.T) {
	type headers struct {
		ID string `header:"X-ID"`
	}
	failure := validation.NewError(validation.FieldError{Field: "/X-ID", Rule: "required"})
	r := &InputDTOResolver{Validator: validatorFunc(func(context.Context, any) error { return failure })}
	_, err := r.Resolve(newFakeHttpCtx(), ParameterMeta{Type: reflect.TypeFor[*headers]()})
	if !errors.Is(err, failure) {
		t.Fatalf("검증 계약의 오류를 보존해야 합니다: %v", err)
	}
}
