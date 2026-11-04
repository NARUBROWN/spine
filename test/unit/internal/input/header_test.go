package input

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestHeaderListsPreserveProtocolMemberBoundaries(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  []string
		want []string
	}{
		{"X-Roles", []string{"reader, writer", "admin"}, []string{"reader", "writer", "admin"}},
		{"X-Roles", []string{`"a,b", c`}, []string{`"a,b"`, "c"}},
		{"X-Roles", []string{`"a\",b", c`}, []string{`"a\",b"`, "c"}},
		{"If-Match", []string{`"a,b", "c"`}, []string{`"a,b"`, `"c"`}},
		{"If-None-Match", []string{`W/"a,b"`, `"c"`}, []string{`W/"a,b"`, `"c"`}},
		{"If-Match", []string{`"a\", "b"`}, []string{`"a\"`, `"b"`}},
		{"Link", []string{`<https://example.com/a,b>; rel="a,b", <https://example.com/c>`}, []string{`<https://example.com/a,b>; rel="a,b"`, `<https://example.com/c>`}},
		{"Via", []string{`1.1 proxy (a,b (c,d)), 1.1 other`}, []string{`1.1 proxy (a,b (c,d))`, `1.1 other`}},
		{"X-Roles", []string{",, reader, ,writer,"}, []string{"reader", "writer"}},
	} {
		t.Run(test.name+test.raw[0], func(t *testing.T) {
			got, err := headerValues(test.name, reflect.TypeFor[[]string](), test.raw)
			if err != nil || !reflect.DeepEqual(got, test.want) {
				t.Fatalf("헤더 원소 경계: got=%q want=%q err=%v", got, test.want, err)
			}
		})
	}
}

func FuzzHeaderListPreservesQuotedMembers(f *testing.F) {
	f.Add("a,b", " c ")
	f.Add(`a\"b`, "")
	f.Fuzz(func(t *testing.T, first, second string) {
		a, _ := json.Marshal(first)
		b, _ := json.Marshal(second)
		want := []string{string(a), string(b)}
		got, err := headerValues("X-Values", reflect.TypeFor[[]string](), []string{string(a) + ", " + string(b)})
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("따옴표 안의 원소를 변경하면 안 됩니다: got=%q want=%q err=%v", got, want, err)
		}
	})
}

func TestHeaderScalarsCombineOnlyTheirDefinedSyntax(t *testing.T) {
	for _, test := range []struct {
		name      string
		raw, want []string
	}{
		{"Accept-Encoding", []string{"gzip", "br"}, []string{"gzip, br"}},
		{"Cookie", []string{"a=1", "b=2"}, []string{"a=1; b=2"}},
		{"If-Modified-Since", []string{"Wed, 21 Oct 2015 07:28:00 GMT"}, []string{"Wed, 21 Oct 2015 07:28:00 GMT"}},
	} {
		got, err := headerValues(test.name, reflect.TypeFor[string](), test.raw)
		if err != nil || !reflect.DeepEqual(got, test.want) {
			t.Fatalf("문자열 헤더 전체 값: %s %q %v", test.name, got, err)
		}
	}
	for _, name := range []string{"X-ID", "Authorization", "Content-Type"} {
		if _, err := headerValues(name, reflect.TypeFor[string](), []string{"same", "same"}); err == nil {
			t.Fatalf("단일값 헤더 중복을 거부해야 합니다: %s", name)
		}
	}
	for _, name := range []string{"Host", "Content-Type", "Cookie", "Www-Authenticate"} {
		if err := checkHeaderType(name, reflect.TypeFor[[]string]()); err == nil {
			t.Fatalf("지원하지 않는 목록 선언을 거부해야 합니다: %s", name)
		}
	}
}

func TestHeaderListsRejectBrokenSyntaxWithoutExposingValues(t *testing.T) {
	for _, test := range []struct{ name, raw string }{
		{"X-Roles", `"private,missing-end`},
		{"If-Match", `*, "a"`},
		{"If-Match", `not-quoted`},
		{"If-Match", `"contains space"`},
		{"Link", `<https://example.com/private`},
		{"Via", `1.1 proxy (private`},
	} {
		if _, err := headerValues(test.name, reflect.TypeFor[[]string](), []string{test.raw}); err == nil {
			t.Fatalf("닫히지 않은 문법과 잘못된 목록을 거부해야 합니다: %s", test.name)
		}
	}
}
