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
				(field.Tag.Get("json") == "-" || field.Tag.Get("xml") == "-" ||
					!bodyTypeBindable(field.Type, "xml") || !bodyTypeBindable(field.Type, "json") || excluded(field.Type)) {
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
		if bodyDecoder(pointer, mode) {
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
	return len(index) > 0 && bodyTypeBindable(t, mode)
}

func bodyDecoder(pointer reflect.Type, mode string) bool {
	return pointer.Implements(reflect.TypeFor[encoding.TextUnmarshaler]()) ||
		mode == "json" && pointer.Implements(reflect.TypeFor[json.Unmarshaler]()) ||
		mode == "xml" && pointer.Implements(reflect.TypeFor[xml.Unmarshaler]())
}

func bodyTypeBindable(t reflect.Type, mode string) bool {
	seen := map[reflect.Type]bool{}
	for !seen[t] {
		seen[t] = true
		if t.Kind() == reflect.Pointer {
			t = t.Elem()
			continue
		}
		if bodyDecoder(reflect.PointerTo(t), mode) {
			return true
		}
		switch t.Kind() {
		case reflect.Slice:
			if mode == "json" {
				return true
			}
		case reflect.Array:
			return mode == "json"
		case reflect.Map:
			return mode == "json" && JSONMapKeyBindable(t.Key())
		case reflect.Interface:
			return mode == "json" && t.NumMethod() == 0
		case reflect.Chan, reflect.Func, reflect.Complex64, reflect.Complex128, reflect.UnsafePointer:
			return false
		default:
			return true
		}
		t = t.Elem()
	}
	return true
}

// JSONMapKeyBindable은 encoding/json이 역직렬화할 수 있는 키를 판별합니다.
func JSONMapKeyBindable(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.String, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return true
	default:
		return reflect.PointerTo(t).Implements(reflect.TypeFor[encoding.TextUnmarshaler]())
	}
}

// ActiveUnbindableBodyField는 요청에 포함된 선택 객체의 필수 필드를 현재 형식으로
// 채울 수 없어 그 필드가 비어 있는지 확인합니다.
func ActiveUnbindableBodyField(dto reflect.Value, media string, declaration validation.SchemaDescription) bool {
	if len(declaration.Fields) == 0 {
		return false
	}
	mode := BodySchemaMode(media)
	form, _ := BodyFields(dto.Type(), true)
	for _, field := range declaration.Fields {
		if !field.Required || !optionalPointerAncestor(dto.Type(), field.Index, declaration.Fields) ||
			bodyFieldBindable(dto.Type(), field.Index, mode, form) {
			continue
		}
		value := dto
		for _, index := range field.Index {
			value = reflect.Indirect(value)
			if !value.IsValid() || value.Kind() != reflect.Struct || index < 0 || index >= value.NumField() {
				value = reflect.Value{}
				break
			}
			value = value.Field(index)
		}
		if value.IsValid() && value.IsZero() {
			return true
		}
	}
	return false
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

// BodySchemaMode는 검증에 같은 태그를 쓰는 두 폼의 명세를 구분합니다.
func BodySchemaMode(media string) string {
	if media == "multipart/form-data" {
		return "multipart"
	}
	return BodyFieldTag(media)
}
