package input

import (
	"encoding"
	"fmt"
	"net/url"
	"reflect"
	"strings"
)

// BindQuery는 반복 키와 쉼표 목록을 원시 쿼리의 순서대로 읽습니다.
// 구분한 뒤 URL 디코딩하므로 %2C는 값으로 남으며 공백과 빈 원소도 보존합니다.
func BindQuery(target reflect.Value, fields []Field, rawQuery string) error {
	encoded := encodedQuery(rawQuery)
	values := make(map[string][]string, len(fields))
	for _, field := range fields {
		for _, raw := range encoded[field.Name] {
			parts := []string{raw}
			if queryList(field.Type) {
				parts = strings.Split(raw, ",")
			}
			for _, part := range parts {
				value, err := url.QueryUnescape(part)
				if err != nil {
					return fmt.Errorf("invalid query field %s", field.Name)
				}
				values[field.Name] = append(values[field.Name], value)
			}
		}
	}
	return Bind(target, fields, values, "query")
}

func queryList(t reflect.Type) bool {
	// 필드 전체의 TextUnmarshaler는 단일 입력을 받는 자체 형식입니다.
	return t.Kind() == reflect.Slice && !reflect.PointerTo(t).Implements(reflect.TypeFor[encoding.TextUnmarshaler]())
}

func encodedQuery(raw string) map[string][]string {
	values := map[string][]string{}
	for raw != "" {
		var pair string
		pair, raw, _ = strings.Cut(raw, "&")
		// net/url과 같이 잘못된 키와 인코딩하지 않은 세미콜론을 포함한 쌍은 제외합니다.
		if pair == "" || strings.Contains(pair, ";") {
			continue
		}
		key, value, _ := strings.Cut(pair, "=")
		key, err := url.QueryUnescape(key)
		if err != nil {
			continue
		}
		values[key] = append(values[key], value)
	}
	return values
}
