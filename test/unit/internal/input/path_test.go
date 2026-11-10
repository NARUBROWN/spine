package input

import (
	"errors"
	httpinput "github.com/NARUBROWN/spine/pkg/input"
	"reflect"
	"strings"
	"testing"
)

type pathToken struct{ Text string }

func (v *pathToken) UnmarshalText(raw []byte) error {
	if string(raw) == "bad" {
		return errors.New("secret token value")
	}
	v.Text = string(raw)
	return nil
}

func TestPathFieldsBindByExactNameWithoutDecodingOrSplitting(t *testing.T) {
	type params struct {
		Item  int64     `path:"item"`
		Org   int64     `path:"org"`
		Zero  *int      `path:"zero"`
		Flag  bool      `path:"flag"`
		Token pathToken `path:"token"`
	}
	typ, source, ok := DTO(reflect.TypeFor[httpinput.Path[params]]())
	if !ok || source != "path" {
		t.Fatal("path 태그로 출처를 판별해야 합니다")
	}
	fields, err := Fields(typ, source)
	if err != nil {
		t.Fatal(err)
	}
	var p params
	err = Bind(reflect.ValueOf(&p).Elem(), fields, map[string][]string{"org": {"42"}, "item": {"7"}, "zero": {"0"}, "flag": {"false"}, "token": {"a%2Fb,c"}}, source)
	if err != nil || p.Org != 42 || p.Item != 7 || p.Zero == nil || *p.Zero != 0 || p.Flag || p.Token.Text != "a%2Fb,c" {
		t.Fatalf("이름 바인딩과 단일 값 보존: %+v %v", p, err)
	}
	for _, values := range []map[string][]string{
		{}, {"item": {"1", "2"}}, {"item": {"secret-invalid-number"}},
		{"item": {"1"}, "org": {"2"}, "zero": {"0"}, "flag": {"false"}, "token": {"bad"}},
	} {
		if err := Bind(reflect.ValueOf(&p).Elem(), fields, values, source); err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("누락과 변환 오류를 입력값 없이 반환해야 합니다: %v", err)
		}
	}
}

func TestPathRejectsCollectionsAndMixedSourcesBeforeBinding(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeFor[struct {
			IDs []int `path:"id"`
		}](),
		reflect.TypeFor[struct {
			IDs [2]int `path:"id"`
		}](),
		reflect.TypeFor[struct {
			IDs map[string]int `path:"id"`
		}](),
		reflect.TypeFor[struct {
			ID struct{ Value int } `path:"id"`
		}](),
		reflect.TypeFor[struct {
			ID int `path:"id" query:"id"`
		}](),
		reflect.TypeFor[struct {
			ID      int `path:"id"`
			Missing string
		}](),
		reflect.TypeFor[struct {
			ID    int `path:"id"`
			Other int `path:"id"`
		}](),
	} {
		if _, err := Fields(typ, "path"); err == nil {
			t.Fatalf("지원하지 않는 경로 선언을 거부해야 합니다: %s", typ)
		}
	}
}
