package input

import (
	"reflect"
	"testing"
)

type FormEmbedded struct {
	ID string `form:"id"`
}
type formAnonymous struct{ FormEmbedded }
type formAnonymousPointer struct{ *FormEmbedded }
type formNamed struct {
	Inner FormEmbedded `json:"-"`
}

func TestBodyPlansShareEmbeddedAndNamedFormFields(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeFor[formAnonymous](), reflect.TypeFor[formNamed]()} {
		fields, conflicts := BodyFields(typ, true)
		if !HasForm(typ) || len(conflicts) != 0 || len(fields) != 1 || fields[0].Name != "id" || !reflect.DeepEqual(fields[0].Index, []int{0, 0}) {
			t.Fatalf("폼 필드 판별과 명세가 같은 계획을 사용해야 합니다: %s %+v %v", typ, fields, conflicts)
		}
	}
	if HasForm(reflect.TypeFor[formAnonymousPointer]()) {
		t.Fatal("기본 폼 바인더가 초기화하지 않는 익명 포인터를 폼 필드로 설명하면 안 됩니다")
	}
}

func TestBodyDeclarationsRejectAmbiguousKeysAndHiddenInputSources(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.StructOf([]reflect.StructField{
			{Name: "First", Type: reflect.TypeFor[string](), Tag: `json:"id"`},
			{Name: "Second", Type: reflect.TypeFor[string](), Tag: `json:"id"`},
		}),
		reflect.TypeFor[struct {
			First string `form:"id"`
			Inner FormEmbedded
		}](),
		reflect.TypeFor[struct {
			Hidden struct {
				ID string `query:"id"`
			} `json:"-"`
		}](),
	} {
		if err := CheckBody(typ); err == nil {
			t.Fatalf("본문 선언의 충돌을 거부해야 합니다: %s", typ)
		}
	}
	if err := CheckBody(reflect.TypeFor[struct {
		ID       string
		Optional *string
	}]()); err != nil {
		t.Fatal(err)
	}
}
