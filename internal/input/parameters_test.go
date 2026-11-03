package input

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

type Embedded struct {
	Page *int `query:"page"`
}

type textList []string

func (values *textList) UnmarshalText(raw []byte) error { *values = textList{string(raw)}; return nil }

func TestCustomTextDecoderReceivesTheUnsplitHeaderValue(t *testing.T) {
	type headers struct {
		Values textList `header:"X-Values"`
	}
	fields, err := Fields(reflect.TypeFor[headers](), "header")
	if err != nil {
		t.Fatal(err)
	}
	var value headers
	if err := Bind(reflect.ValueOf(&value).Elem(), fields, map[string][]string{"X-Values": {"a,b"}}, "header"); err != nil || len(value.Values) != 1 || value.Values[0] != "a,b" {
		t.Fatalf("사용자 디코더 입력: %+v %v", value, err)
	}
}

func TestBindingUsesOnlyItsSourceAndPreservesPresence(t *testing.T) {
	type filters struct {
		Embedded
		IDs     []uint8   `query:"id"`
		When    time.Time `query:"when"`
		Ignored string    `json:"ignored"`
	}
	fields, err := Fields(reflect.TypeFor[filters](), "query")
	if err != nil || len(fields) != 3 {
		t.Fatalf("필드 선택: %+v %v", fields, err)
	}
	var result filters
	if err := Bind(reflect.ValueOf(&result).Elem(), fields, map[string][]string{"page": {"0"}, "id": {"1", "255"}, "when": {"2026-11-03T00:00:00Z"}, "ignored": {"hidden"}}, "query"); err != nil {
		t.Fatal(err)
	}
	if result.Page == nil || *result.Page != 0 || len(result.IDs) != 2 || result.When.IsZero() || result.Ignored != "" {
		t.Fatalf("바인딩: %+v", result)
	}
	var absent filters
	if err := Bind(reflect.ValueOf(&absent).Elem(), fields, nil, "query"); err != nil || absent.Page != nil {
		t.Fatalf("누락과 0 구분: %+v %v", absent, err)
	}
	for _, invalid := range []string{"256", "-1", "secret-value"} {
		if err := Bind(reflect.ValueOf(&result).Elem(), fields, map[string][]string{"id": {invalid}}, "query"); err == nil || strings.Contains(err.Error(), invalid) {
			t.Fatalf("범위 검사와 입력값 비노출: %v", err)
		}
	}
}

func TestHeaderListsAndInvalidFieldPlans(t *testing.T) {
	type headers struct {
		Values []string `header:"X-Values"`
	}
	fields, err := Fields(reflect.TypeFor[headers](), "header")
	if err != nil {
		t.Fatal(err)
	}
	var result headers
	if err := Bind(reflect.ValueOf(&result).Elem(), fields, map[string][]string{"X-Values": {"a, b", "c"}}, "header"); err != nil || len(result.Values) != 3 {
		t.Fatalf("헤더 배열: %+v %v", result, err)
	}
	for _, typ := range []reflect.Type{
		reflect.TypeFor[struct {
			A string `header:"X-ID"`
			B string `header:"x-id"`
		}](),
		reflect.TypeFor[struct {
			A string `header:"X ID"`
		}](),
		reflect.TypeFor[struct {
			A map[string]string `header:"X-ID"`
		}](),
	} {
		if _, err := Fields(typ, "header"); err == nil {
			t.Fatalf("잘못된 입력 계약을 거부해야 합니다: %s", typ)
		}
	}
}

func TestOrdinaryDTOClassificationAndMixedSources(t *testing.T) {
	type headers struct {
		ID string `header:"X-ID"`
	}
	typ, source, ok := DTO(reflect.TypeFor[*headers]())
	if !ok || source != "header" || typ != reflect.TypeFor[headers]() {
		t.Fatalf("일반 헤더 DTO 식별: %s %s %v", typ, source, ok)
	}
	for _, typ := range []reflect.Type{
		reflect.TypeFor[struct {
			Q  string `query:"q"`
			ID string `header:"X-ID"`
		}](),
		reflect.TypeFor[struct {
			Q    string `query:"q"`
			Name string `form:"name"`
		}](),
		reflect.TypeFor[struct{ *Embedded }](),
	} {
		if _, err := Fields(typ, "query"); err == nil {
			t.Fatalf("혼합 출처와 익명 포인터를 거부해야 합니다: %s", typ)
		}
	}
}
