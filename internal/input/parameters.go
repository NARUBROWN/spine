// Package input은 타입으로 출처를 지정한 입력의 바인딩과 설명 규칙을 공유합니다.
package input

import (
	"encoding"
	"fmt"
	"math"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type Field struct {
	Name  string
	Index []int
	Type  reflect.Type
}

// DTO는 일반 구조체의 query 또는 header 태그에서 입력 출처를 식별합니다.
// 타입 이름과 제네릭 래퍼에 의존하지 않습니다. 혼합 출처는 Fields에서 거부합니다.
func DTO(t reflect.Type) (reflect.Type, string, bool) {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, "", false
	}
	visited := map[reflect.Type]bool{}
	var source func(reflect.Type) string
	source = func(current reflect.Type) string {
		if visited[current] {
			return ""
		}
		visited[current] = true
		for i := 0; i < current.NumField(); i++ {
			field := current.Field(i)
			if !field.IsExported() {
				continue
			}
			for _, name := range []string{"query", "header"} {
				if tag := field.Tag.Get(name); tag != "" && tag != "-" {
					return name
				}
			}
			base := field.Type
			if base.Kind() == reflect.Pointer {
				base = base.Elem()
			}
			if field.Anonymous && base.Kind() == reflect.Struct {
				if name := source(base); name != "" {
					return name
				}
			}
		}
		return ""
	}
	name := source(t)
	return t, name, name != ""
}

// Fields는 명시한 출처의 태그만 선택합니다. 중복 이름과 지원하지 않는 타입은 거부합니다.
func Fields(t reflect.Type, source string) ([]Field, error) {
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("%s DTO must contain a struct", source)
	}
	var fields []Field
	seen := map[string]bool{}
	visiting := map[reflect.Type]bool{}
	var walk func(reflect.Type, []int) error
	walk = func(current reflect.Type, prefix []int) error {
		if visiting[current] {
			return fmt.Errorf("recursive anonymous %s DTO", source)
		}
		visiting[current] = true
		defer delete(visiting, current)
		for i := 0; i < current.NumField(); i++ {
			f := current.Field(i)
			if !f.IsExported() {
				continue
			}
			for _, other := range []string{"query", "header", "form", "param"} {
				if tag := f.Tag.Get(other); other != source && tag != "" && tag != "-" {
					return fmt.Errorf("%s DTO contains %s input; use separate handler parameters", source, other)
				}
			}
			index := append(append([]int(nil), prefix...), i)
			name := f.Tag.Get(source)
			base := f.Type
			if base.Kind() == reflect.Pointer {
				base = base.Elem()
			}
			if name == "" && f.Anonymous && base.Kind() == reflect.Struct {
				if f.Type.Kind() == reflect.Pointer {
					return fmt.Errorf("anonymous pointer fields are not supported in %s DTOs", source)
				}
				if err := walk(base, index); err != nil {
					return err
				}
				continue
			}
			if name == "" || name == "-" {
				continue
			}
			if strings.Contains(name, ",") || strings.IndexFunc(name, unicode.IsControl) >= 0 {
				return fmt.Errorf("invalid %s field name %q", source, name)
			}
			if source == "header" {
				for _, r := range name {
					if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", r)) {
						return fmt.Errorf("invalid header field name %q", name)
					}
				}
				name = http.CanonicalHeaderKey(name)
			}
			if seen[name] {
				return fmt.Errorf("duplicate %s field %q", source, name)
			}
			if !supported(f.Type) {
				return fmt.Errorf("unsupported %s field %s: %s", source, name, f.Type)
			}
			seen[name] = true
			fields = append(fields, Field{Name: name, Index: index, Type: f.Type})
		}
		return nil
	}
	err := walk(t, nil)
	return fields, err
}

func supported(t reflect.Type) bool {
	if reflect.PointerTo(t).Implements(reflect.TypeFor[encoding.TextUnmarshaler]()) {
		return true
	}
	if t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice {
		return supportedScalar(t.Elem())
	}
	return supportedScalar(t)
}

func supportedScalar(t reflect.Type) bool {
	if reflect.PointerTo(t).Implements(reflect.TypeFor[encoding.TextUnmarshaler]()) || t == reflect.TypeFor[time.Time]() {
		return true
	}
	return t.Kind() == reflect.Bool || t.Kind() == reflect.String || t.Kind() >= reflect.Int && t.Kind() <= reflect.Float64
}

// Bind는 이미 선택한 필드 목록으로만 값을 읽습니다. 사용자 코드는 요청 처리 중에만 호출합니다.
func Bind(target reflect.Value, fields []Field, values map[string][]string, source string) error {
	for _, field := range fields {
		raw, exists := values[field.Name]
		if !exists || len(raw) == 0 {
			continue
		}
		value := target
		for _, index := range field.Index {
			if value.Kind() == reflect.Pointer {
				if value.IsNil() {
					value.Set(reflect.New(value.Type().Elem()))
				}
				value = value.Elem()
			}
			value = value.Field(index)
		}
		if source == "header" && value.Kind() == reflect.Slice && !value.Addr().Type().Implements(reflect.TypeFor[encoding.TextUnmarshaler]()) {
			var parts []string
			for _, line := range raw {
				for _, part := range strings.Split(line, ",") {
					parts = append(parts, strings.TrimSpace(part))
				}
			}
			raw = parts
		}
		if err := set(value, raw); err != nil {
			// 거부한 입력값과 사용자 UnmarshalText 오류 원문은 노출하지 않습니다.
			return fmt.Errorf("invalid %s field %s", source, field.Name)
		}
	}
	return nil
}

func set(value reflect.Value, raw []string) error {
	if value.Kind() == reflect.Pointer {
		value.Set(reflect.New(value.Type().Elem()))
		return set(value.Elem(), raw)
	}
	if value.Type() == reflect.TypeFor[time.Time]() {
		parsed, err := time.Parse(time.RFC3339Nano, raw[0])
		if err == nil {
			value.Set(reflect.ValueOf(parsed))
		}
		return err
	}
	if decoder, ok := value.Addr().Interface().(encoding.TextUnmarshaler); ok {
		return decoder.UnmarshalText([]byte(raw[0]))
	}
	if value.Kind() == reflect.Slice {
		value.Set(reflect.MakeSlice(value.Type(), len(raw), len(raw)))
		for i, item := range raw {
			if err := set(value.Index(i), []string{item}); err != nil {
				return err
			}
		}
		return nil
	}
	switch value.Kind() {
	case reflect.String:
		value.SetString(raw[0])
	case reflect.Bool:
		parsed, err := strconv.ParseBool(raw[0])
		if err != nil {
			return err
		}
		value.SetBool(parsed)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		parsed, err := strconv.ParseInt(raw[0], 10, value.Type().Bits())
		if err != nil {
			return err
		}
		value.SetInt(parsed)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		parsed, err := strconv.ParseUint(raw[0], 10, value.Type().Bits())
		if err != nil {
			return err
		}
		value.SetUint(parsed)
	case reflect.Float32, reflect.Float64:
		parsed, err := strconv.ParseFloat(raw[0], value.Type().Bits())
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return fmt.Errorf("invalid number")
		}
		value.SetFloat(parsed)
	}
	return nil
}
