package input

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// CheckBody는 태그 없는 JSON 필드의 기본 이름을 허용하면서 다른 입력 출처의 혼합을 검사합니다.
// 사용자 정의 JSON 디코더의 내부 필드는 해당 구현이 소유합니다.
func CheckBody(t reflect.Type) error {
	seen := map[reflect.Type]bool{}
	var walk func(reflect.Type) error
	walk = func(current reflect.Type) error {
		if seen[current] {
			return nil
		}
		seen[current] = true
		if current.Implements(reflect.TypeFor[json.Unmarshaler]()) || reflect.PointerTo(current).Implements(reflect.TypeFor[json.Unmarshaler]()) {
			return nil
		}
		switch current.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
			return walk(current.Elem())
		case reflect.Struct:
			for _, form := range []bool{false, true} {
				_, conflicts := BodyFields(current, form)
				if len(conflicts) > 0 {
					return fmt.Errorf("body DTO %s repeats input key %q", current, conflicts[0])
				}
			}
			for i := 0; i < current.NumField(); i++ {
				field := current.Field(i)
				if !field.IsExported() {
					if field.Anonymous {
						if err := walk(field.Type); err != nil {
							return err
						}
					}
					continue
				}
				if err := CheckTagNames(field); err != nil {
					return err
				}
				for _, source := range []string{"query", "header", "param"} {
					if tag, exists := field.Tag.Lookup(source); exists && tag != "-" {
						return fmt.Errorf("body DTO field %s contains %s input; use separate handler parameters", field.Name, source)
					}
				}
				if err := walk(field.Type); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk(t)
}

// HasForm은 중첩 값과 익명 임베딩에 있는 폼 필드도 같은 기준으로 찾습니다.
func HasForm(t reflect.Type) bool {
	fields, conflicts := BodyFields(t, true)
	return len(fields) > 0 || len(conflicts) > 0
}

// CheckTagNames는 지원하는 입력 태그의 흔한 복수형 오타가 본문으로 처리되지 않도록 합니다.
// 알 수 없는 일반 메타데이터 태그를 입력 출처로 추측하지 않습니다.
func CheckTagNames(field reflect.StructField) error {
	for _, name := range []string{"headers", "queries", "querys", "params", "forms"} {
		if _, exists := field.Tag.Lookup(name); exists {
			return fmt.Errorf("field %s uses unsupported input tag %q; use json, query, header, form or param", field.Name, name)
		}
	}
	return nil
}
