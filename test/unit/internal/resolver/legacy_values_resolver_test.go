package resolver

import (
	"errors"
	"reflect"
	"testing"

	"github.com/NARUBROWN/spine/pkg/header"
	"github.com/NARUBROWN/spine/pkg/query"
)

func TestLegacyValuesResolversInjectViewsWithoutBodyBinding(t *testing.T) {
	ctx := newFakeHttpCtx()
	ctx.bindErr = errors.New("기존 조회는 본문 바인더를 실행하면 안 됩니다")
	ctx.queries["count"] = []string{"invalid", "2"}
	ctx.headers["X-Id"] = []string{"first", "second"}
	for _, test := range []struct {
		resolver ArgumentResolver
		typ      reflect.Type
	}{
		{&QueryValuesResolver{}, reflect.TypeFor[query.Values]()},
		{&HeaderResolver{}, reflect.TypeFor[header.Values]()},
	} {
		pm := ParameterMeta{Type: test.typ}
		if !test.resolver.Supports(pm) || test.resolver.Supports(ParameterMeta{Type: reflect.PointerTo(test.typ)}) {
			t.Fatalf("기존 타입은 값 인자로만 주입합니다: %v", test.typ)
		}
		value, err := test.resolver.Resolve(ctx, pm)
		if err != nil {
			t.Fatal(err)
		}
		switch value := value.(type) {
		case query.Values:
			if value.Int("count", 7) != 7 || value.Get("count") != "invalid" {
				t.Fatal("기존 쿼리의 변환 실패와 첫 번째 값을 유지해야 합니다")
			}
		case header.Values:
			if value.Get("x-id") != "first" || !value.Has("X-ID") {
				t.Fatal("기존 헤더의 조회 규칙을 유지해야 합니다")
			}
		default:
			t.Fatalf("주입 타입: %T", value)
		}
	}
}
