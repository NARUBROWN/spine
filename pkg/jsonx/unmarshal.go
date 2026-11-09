// Package jsonx는 Spine 입력에서 일반 값 타입의 JSON null을 거부합니다.
package jsonx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
)

// NullError는 nil을 표현할 수 없는 일반 값 타입에 JSON null을 전달한 오류입니다.
// 입력값은 포함하지 않습니다. 누락된 필드와 검증 규칙 위반은 이 오류가 아닙니다.
type NullError struct{ Type reflect.Type }

func (e *NullError) Error() string {
	return fmt.Sprintf("JSON null cannot be decoded into %s", e.Type)
}

// Unmarshal은 encoding/json.Unmarshal의 바인딩에 앞서 일반 값의 null을 검사합니다.
// 포인터, map, slice와 interface의 null은 허용하며 필수 여부는 검증기에 맡깁니다.
// json.Unmarshaler를 구현한 타입은 자신의 null 처리 규칙을 사용합니다.
func Unmarshal(data []byte, target any) error {
	t := reflect.TypeOf(target)
	if t == nil || t.Kind() != reflect.Pointer || reflect.ValueOf(target).IsNil() || !mayContainNull(data) || !json.Valid(data) {
		return json.Unmarshal(data, target)
	}
	if err := checkNull(data, t.Elem(), false); err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func checkNull(data []byte, t reflect.Type, quoted bool) error {
	data = bytes.TrimSpace(data)
	if !mayContainNull(data) || t.Implements(reflect.TypeFor[json.Unmarshaler]()) || reflect.PointerTo(t).Implements(reflect.TypeFor[json.Unmarshaler]()) {
		return nil
	}
	if bytes.Equal(data, []byte("null")) {
		switch t.Kind() {
		case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Interface:
			return nil
		}
		return &NullError{Type: t}
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
		if t.Implements(reflect.TypeFor[json.Unmarshaler]()) || reflect.PointerTo(t).Implements(reflect.TypeFor[json.Unmarshaler]()) {
			return nil
		}
	}
	if quoted && data[0] == '"' {
		var inner string
		if err := json.Unmarshal(data, &inner); err != nil {
			return err
		}
		return checkNull([]byte(inner), t, false)
	}
	if data[0] == '{' && (t.Kind() == reflect.Struct || t.Kind() == reflect.Map) {
		decoder := json.NewDecoder(bytes.NewReader(data))
		_, _ = decoder.Token()
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			var raw json.RawMessage
			if err := decoder.Decode(&raw); err != nil {
				return err
			}
			child, childQuoted := t, false
			if t.Kind() == reflect.Map {
				child = t.Elem()
			} else {
				field, ok := fieldsFor(t).find(key.(string))
				if !ok {
					continue
				}
				child, childQuoted = field.typeOf, field.quoted
			}
			if err := checkNull(raw, child, childQuoted); err != nil {
				return err
			}
		}
	}
	if data[0] == '[' && (t.Kind() == reflect.Array || t.Kind() == reflect.Slice) {
		decoder := json.NewDecoder(bytes.NewReader(data))
		_, _ = decoder.Token()
		for index := 0; decoder.More(); index++ {
			var raw json.RawMessage
			if err := decoder.Decode(&raw); err != nil {
				return err
			}
			// encoding/json은 고정 배열의 길이를 넘는 입력 원소를 버립니다.
			if t.Kind() == reflect.Array && index >= t.Len() {
				continue
			}
			if err := checkNull(raw, t.Elem(), false); err != nil {
				return err
			}
		}
	}
	return nil
}

func mayContainNull(data []byte) bool {
	// ,string 필드는 유니코드 이스케이프로 표현한 내부 null도 검사합니다.
	return bytes.Contains(data, []byte("null")) || bytes.Contains(data, []byte(`\u`))
}
