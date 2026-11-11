package input

import (
	"encoding/xml"
	"errors"
	"reflect"
	"testing"

	httpinput "github.com/NARUBROWN/spine/pkg/input"
)

type XMLBodyEmbedded struct {
	Value string `xml:"value"`
}

type XMLBodyOtherEmbedded struct {
	Other string `xml:"value>child"`
}

type xmlBodyNamedChild struct {
	XMLName xml.Name `xml:"child"`
}

func TestXMLBodyConflictsMatchStandardFieldResolution(t *testing.T) {
	pair := func(a, b string) reflect.Type {
		return reflect.StructOf([]reflect.StructField{
			{Name: "A", Type: reflect.TypeFor[string](), Tag: reflect.StructTag(`xml:"` + a + `"`)},
			{Name: "B", Type: reflect.TypeFor[string](), Tag: reflect.StructTag(`xml:"` + b + `"`)},
		})
	}
	for _, tc := range []struct {
		name     string
		typ      reflect.Type
		conflict bool
	}{
		{"duplicate_elements", pair("same", "same"), true},
		{"default_name", pair("", "A"), true},
		{"same_parent", pair("parent>a", "parent>b"), false},
		{"duplicate_paths", pair("parent>a", "parent>a"), true},
		{"parent_before_child", pair("parent", "parent>child"), true},
		{"child_before_parent", pair("parent>child", "parent"), true},
		{"element_and_attribute", pair("same", "same,attr"), false},
		{"duplicate_attributes", pair("same,attr", "same,attr"), true},
		{"same_namespace", pair("urn:a same", "urn:a same"), true},
		{"different_namespaces", pair("urn:a same", "urn:b same"), false},
		{"unqualified_and_qualified", pair("same", "urn:a same"), false},
		{"qualified_parent_conflict", pair("parent", "urn:a parent>child"), true},
		{"different_qualified_parents", pair("urn:a parent", "urn:b parent>child"), false},
		{"ignored", pair("same", "-"), false},
		{"omitempty", pair("same", "same,omitempty"), true},
		{"unknown_option", pair("same", "same,unknown"), true},
		{"embedded_shadowed", reflect.TypeFor[struct {
			XMLBodyEmbedded
			Local string `xml:"value"`
		}](), false},
		{"embedded_shadowed_reversed", reflect.TypeFor[struct {
			Local string `xml:"value"`
			*XMLBodyEmbedded
		}](), false},
		{"embedded_same_depth", reflect.TypeFor[struct {
			XMLBodyEmbedded
			XMLBodyOtherEmbedded
		}](), true},
		{"child_xmlname_defaults", reflect.TypeFor[struct {
			First  xmlBodyNamedChild
			Second xmlBodyNamedChild
		}](), true},
		{"child_xmlname_explicit", reflect.TypeFor[struct {
			First  xmlBodyNamedChild
			Second string `xml:"other"`
		}](), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var standard, checked *xml.TagPathError
			standardErr := xml.Unmarshal([]byte("<root/>"), reflect.New(tc.typ).Interface())
			checkedErr := checkXMLBody(tc.typ)
			if errors.As(standardErr, &standard) != tc.conflict || errors.As(checkedErr, &checked) != tc.conflict {
				t.Fatalf("XML 경로 충돌 판정이 다릅니다: standard=%v check=%v", standardErr, checkedErr)
			}
		})
	}
}

type xmlBodyCollision struct {
	First  string `json:"first" xml:"same"`
	Second string `json:"second" xml:"same>child"`
}

type xmlBodyCustomDecoder xmlBodyCollision

func (*xmlBodyCustomDecoder) UnmarshalXML(*xml.Decoder, xml.StartElement) error {
	panic("선언 검사에서 사용자 XML 디코더를 실행하면 안 됩니다")
}

type xmlBodyTextDecoder xmlBodyCollision

func (*xmlBodyTextDecoder) UnmarshalText([]byte) error {
	panic("선언 검사에서 사용자 텍스트 디코더를 실행하면 안 됩니다")
}

type xmlBodyJSONDecoder xmlBodyCollision

func (*xmlBodyJSONDecoder) UnmarshalJSON([]byte) error {
	panic("선언 검사에서 사용자 JSON 디코더를 실행하면 안 됩니다")
}

type xmlBodyRecursive struct {
	Next *xmlBodyRecursive `json:"next" xml:"next"`
}

func TestBodyInspectionRejectsXMLConflictsWithoutExecutingDecoders(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeFor[httpinput.Body[xmlBodyCollision]](),
		reflect.TypeFor[httpinput.Body[xmlBodyJSONDecoder]](),
		reflect.TypeFor[httpinput.Body[struct{ Child *xmlBodyCollision }]](),
		reflect.TypeFor[httpinput.Body[struct{ Children []xmlBodyCollision }]](),
	} {
		_, ok, err := Inspect(typ)
		var conflict *xml.TagPathError
		if !ok || !errors.As(err, &conflict) {
			t.Errorf("XML 충돌을 선언 검사에서 거부해야 합니다: %s: %v", typ, err)
		}
	}
	for _, typ := range []reflect.Type{
		reflect.TypeFor[httpinput.Body[xmlBodyCustomDecoder]](),
		reflect.TypeFor[httpinput.Body[xmlBodyTextDecoder]](),
		reflect.TypeFor[httpinput.Body[struct{ Child xmlBodyCustomDecoder }]](),
		reflect.TypeFor[httpinput.Body[struct{ Child xmlBodyTextDecoder }]](),
		reflect.TypeFor[httpinput.Body[struct {
			Child xmlBodyCollision `xml:"-"`
		}]](),
		reflect.TypeFor[httpinput.Body[xmlBodyRecursive]](),
	} {
		if _, ok, err := Inspect(typ); !ok || err != nil {
			t.Errorf("사용자 디코더와 유효한 재귀 타입을 거부하면 안 됩니다: %s: %v", typ, err)
		}
	}
}
