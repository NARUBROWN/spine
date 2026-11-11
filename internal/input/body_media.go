package input

import (
	"encoding"
	"encoding/json"
	"encoding/xml"
	"reflect"
	"slices"

	validation "github.com/NARUBROWN/spine/contract/validator"
)

// BodyMediaNeedsValidation은 필수 규칙이 형식 지원을 바꿀 수 있는 DTO만 가려냅니다.
func BodyMediaNeedsValidation(t reflect.Type) bool {
	if HasForm(t) {
		return true
	}
	seen := map[reflect.Type]bool{}
	var excluded func(reflect.Type) bool
	excluded = func(t reflect.Type) bool {
		if t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct || seen[t] {
			return false
		}
		seen[t] = true
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if (field.IsExported() || field.Anonymous) &&
				(field.Tag.Get("json") == "-" || field.Tag.Get("xml") == "-" || excluded(field.Type)) {
				return true
			}
		}
		return false
	}
	return excluded(t)
}

// BodyMediaTypes는 필수 필드를 모두 바인딩할 수 있는 본문 형식을 반환합니다.
func BodyMediaTypes(t reflect.Type, declaration validation.SchemaDescription) []string {
	form, _ := BodyFields(t, true)
	required := make([][]int, 0, len(declaration.Fields))
	for _, field := range declaration.Fields {
		if field.Required && !optionalPointerAncestor(t, field.Index, declaration.Fields) {
			required = append(required, field.Index)
		}
	}
	available := func(mode string) bool {
		for _, index := range required {
			if !bodyFieldBindable(t, index, mode, form) {
				return false
			}
		}
		return true
	}
	var media []string
	if available("json") {
		media = append(media, "application/json")
	}
	if available("xml") {
		media = append(media, "application/xml", "text/xml")
	}
	if len(form) > 0 {
		if available("form") {
			media = append(media, "application/x-www-form-urlencoded")
		}
		if available("multipart") {
			media = append(media, "multipart/form-data")
		}
	}
	return media
}

func optionalPointerAncestor(t reflect.Type, index []int, constraints []validation.FieldSchema) bool {
	for depth, position := range index {
		if t.Kind() != reflect.Struct || position < 0 || position >= t.NumField() {
			return false
		}
		t = t.Field(position).Type
		if t.Kind() == reflect.Pointer && depth < len(index)-1 {
			if !slices.ContainsFunc(constraints, func(field validation.FieldSchema) bool {
				return field.Required && slices.Equal(field.Index, index[:depth+1])
			}) {
				return true
			}
			t = t.Elem()
		}
	}
	return false
}

func bodyFieldBindable(t reflect.Type, index []int, mode string, form []BodyField) bool {
	if mode == "form" || mode == "multipart" {
		for _, field := range form {
			if len(field.Index) >= len(index) && slices.Equal(field.Index[:len(index)], index) ||
				len(field.Index) < len(index) && slices.Equal(index[:len(field.Index)], field.Index) {
				if mode == "multipart" || !FormFileType(field.Type) {
					return true
				}
			}
		}
		return false
	}

	for depth, position := range index {
		if t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct || position < 0 || position >= t.NumField() {
			return false
		}
		pointer := reflect.PointerTo(t)
		if pointer.Implements(reflect.TypeFor[encoding.TextUnmarshaler]()) ||
			mode == "json" && pointer.Implements(reflect.TypeFor[json.Unmarshaler]()) ||
			mode == "xml" && pointer.Implements(reflect.TypeFor[xml.Unmarshaler]()) {
			return true
		}
		field := t.Field(position)
		if !field.IsExported() && !field.Anonymous || field.Tag.Get(mode) == "-" {
			return false
		}
		if depth == len(index)-1 {
			for _, formField := range form {
				if slices.Equal(formField.Index, index) && FormFileType(formField.Type) {
					return false
				}
			}
		}
		t = field.Type
	}
	return len(index) > 0
}

// BodyFieldTag는 요청 형식을 검증 오류의 필드 태그와 연결합니다.
func BodyFieldTag(media string) string {
	switch media {
	case "application/x-www-form-urlencoded", "multipart/form-data":
		return "form"
	case "application/xml", "text/xml":
		return "xml"
	default:
		return "json"
	}
}
