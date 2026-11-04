package input

import (
	"net/url"
	"reflect"
	"strings"
	"testing"
)

type listQuery struct {
	Tags []string `query:"tags"`
}

func TestBindQuerySeparatesBeforeURLDecodingWithoutTrimming(t *testing.T) {
	fields, err := Fields(reflect.TypeFor[listQuery](), "query")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		raw  string
		want []string
	}{
		{"", nil},
		{"tags=a&tags=b", []string{"a", "b"}},
		{"tags=a,b", []string{"a", "b"}},
		{"tags=a,b&tags=c,d", []string{"a", "b", "c", "d"}},
		{"tags=a&tags=a,a", []string{"a", "a", "a"}},
		{"tags=a,%20b", []string{"a", " b"}},
		{"tags=%20a%20,b+", []string{" a ", "b "}},
		{"tags=a%2Cb", []string{"a,b"}},
		{"tags=a%2cb,c", []string{"a,b", "c"}},
		{"tags=%252C", []string{"%2C"}},
		{"tags=a%26b%3Dc,%2B", []string{"a&b=c", "+"}},
		{"tags=&tags=,a", []string{"", "", "a"}},
		{"tags=a&%74ags=b,c", []string{"a", "b", "c"}},
		{"other=%zz&tags=x", []string{"x"}},
		{"%zz=a&tags=x", []string{"x"}},
		{"tags=a;b&tags=c%3Bd", []string{"c;d"}},
	} {
		t.Run(test.raw, func(t *testing.T) {
			var value listQuery
			if err := BindQuery(reflect.ValueOf(&value).Elem(), fields, test.raw); err != nil || !reflect.DeepEqual(value.Tags, test.want) {
				t.Fatalf("원시 쿼리 분리와 디코딩: got=%q want=%q err=%v", value.Tags, test.want, err)
			}
		})
	}
}

type textAtom string

func (v *textAtom) UnmarshalText(raw []byte) error { *v = textAtom(raw); return nil }

func TestBindQueryPreservesScalarAndCustomTextFormats(t *testing.T) {
	type query struct {
		Name  string     `query:"name"`
		Whole textList   `query:"whole"`
		Atoms []textAtom `query:"atom"`
	}
	fields, err := Fields(reflect.TypeFor[query](), "query")
	if err != nil {
		t.Fatal(err)
	}
	var value query
	if err := BindQuery(reflect.ValueOf(&value).Elem(), fields, "name=a,b&whole=a,b&atom=a%2Cb,c"); err != nil {
		t.Fatal(err)
	}
	if value.Name != "a,b" || !reflect.DeepEqual(value.Whole, textList{"a,b"}) || !reflect.DeepEqual(value.Atoms, []textAtom{"a,b", "c"}) {
		t.Fatalf("일반 문자열과 사용자 디코더의 값 경계를 보존해야 합니다: %+v", value)
	}
	for _, raw := range []string{"name=a&name=a", "whole=a&whole=b"} {
		if err := BindQuery(reflect.ValueOf(&value).Elem(), fields, raw); err == nil {
			t.Fatalf("단일 입력은 반복 키를 거부해야 합니다: %s", raw)
		}
	}
}

func TestBindQueryRejectsInvalidItemsAndRepeatedScalarPointers(t *testing.T) {
	type query struct {
		IDs   []uint8  `query:"id"`
		Count *int     `query:"count"`
		Tags  []string `query:"tags"`
	}
	fields, err := Fields(reflect.TypeFor[query](), "query")
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		"id=1,256", "id=1&id=-1", "id=1,%202", "id=1%2C2", "id=",
		"count=0&count=0", "count=&count=1", "tags=%zz",
	} {
		var value query
		if err := BindQuery(reflect.ValueOf(&value).Elem(), fields, raw); err == nil || strings.Contains(err.Error(), "%zz") {
			t.Fatalf("잘못된 원소와 단일 값 중복을 입력값 노출 없이 거부해야 합니다: %s %v", raw, err)
		}
	}
	var value query
	if err := BindQuery(reflect.ValueOf(&value).Elem(), fields, "id=1,%32&count=0"); err != nil || !reflect.DeepEqual(value.IDs, []uint8{1, 2}) || value.Count == nil || *value.Count != 0 {
		t.Fatalf("숫자도 분리 후 변환하고 실제 0을 보존해야 합니다: %+v %v", value, err)
	}
}

func FuzzBindQueryPreservesEncodedItemBoundaries(f *testing.F) {
	for _, pair := range [][2]string{{"a,b", " c"}, {"", ""}, {"a&b=c", "+%2C"}, {"가", "나"}} {
		f.Add(pair[0], pair[1])
	}
	fields, err := Fields(reflect.TypeFor[listQuery](), "query")
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, first, second string) {
		for _, raw := range []string{
			url.Values{"tags": {first, second}}.Encode(),
			"tags=" + url.QueryEscape(first) + "," + url.QueryEscape(second),
		} {
			var value listQuery
			if err := BindQuery(reflect.ValueOf(&value).Elem(), fields, raw); err != nil || !reflect.DeepEqual(value.Tags, []string{first, second}) {
				t.Fatalf("인코딩한 원소가 다른 값으로 분리되면 안 됩니다: raw=%q got=%q err=%v", raw, value.Tags, err)
			}
		}
	})
}
