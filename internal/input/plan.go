package input

import (
	"fmt"
	"reflect"
	"strings"
	"unicode"
)

// Plan은 바인딩, 준비 검사와 명세가 공유하는 입력 타입 해석입니다.
type Plan struct {
	Declared reflect.Type
	Payload  reflect.Type
	Source   string
	Fields   []Field
	Wrapped  bool
	Scalar   bool
}

func Wrapper(t reflect.Type) (string, bool) {
	if t == nil || t.PkgPath() != "github.com/NARUBROWN/spine/pkg/input" {
		return "", false
	}
	for _, entry := range []struct{ name, source string }{{"Path[", "path"}, {"Query[", "query"}, {"Header[", "header"}, {"Body[", "body"}} {
		if strings.HasPrefix(t.Name(), entry.name) {
			return entry.source, true
		}
	}
	return "", false
}

func Inspect(t reflect.Type) (Plan, bool, error) {
	source, wrapped := Wrapper(t)
	plan := Plan{Declared: t, Source: source, Wrapped: wrapped}
	if wrapped {
		if t.Kind() == reflect.Pointer && source == "path" && scalarPathType(t.Elem()) {
			plan.Payload, plan.Scalar = t.Elem(), true
			return plan, true, nil
		}
		if t.Kind() != reflect.Pointer || t.Elem().Kind() != reflect.Struct {
			return plan, true, fmt.Errorf("input.%s requires a struct payload", t.Name())
		}
		plan.Payload = t.Elem()
	} else {
		payload, name, ok := DTO(t)
		if !ok {
			return plan, false, nil
		}
		plan.Payload, plan.Source = payload, name
	}
	if plan.Source == "body" {
		return plan, true, CheckBody(plan.Payload)
	}
	fields, err := Fields(plan.Payload, plan.Source, wrapped)
	plan.Fields = fields
	return plan, true, err
}

// LowerCamel은 선행 약어를 포함한 필드 이름을 외부 기본 키로 변환합니다.
func LowerCamel(name string) string {
	runes := []rune(name)
	var words []string
	start := 0
	for index := 1; index < len(runes); index++ {
		if unicode.IsUpper(runes[index]) && (unicode.IsLower(runes[index-1]) || unicode.IsDigit(runes[index-1]) || unicode.IsUpper(runes[index-1]) && index+1 < len(runes) && unicode.IsLower(runes[index+1])) {
			words = append(words, strings.ToLower(string(runes[start:index])))
			start = index
		}
	}
	words = append(words, strings.ToLower(string(runes[start:])))
	for index := 1; index < len(words); index++ {
		chars := []rune(words[index])
		if len(chars) > 0 {
			chars[0] = unicode.ToUpper(chars[0])
		}
		words[index] = string(chars)
	}
	return strings.Join(words, "")
}

// FieldName은 익명 임베딩의 실제 소유 타입까지 대조하여 검증 오류 이름을 매핑합니다.
func (p Plan) FieldName(owner reflect.Type, field reflect.StructField) (string, bool) {
	for _, bound := range p.Fields {
		current := p.Payload
		for position, index := range bound.Index {
			if position == len(bound.Index)-1 {
				if current == owner && len(field.Index) == 1 && index == field.Index[0] {
					return bound.Name, true
				}
				break
			}
			current = current.Field(index).Type
		}
	}
	return "", false
}

// scalarPathType는 명세와 변환을 일치시킬 수 있는 기본 스칼라만 허용합니다.
func scalarPathType(t reflect.Type) bool {
	if t.PkgPath() != "" {
		return false
	}
	switch t.Kind() {
	case reflect.String, reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr, reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

// BindScalarPath는 DTO 필드와 같은 변환 규칙과 오류 형태를 사용합니다.
func BindScalarPath(target reflect.Value, name, raw string) error {
	if err := set(target, []string{raw}); err != nil {
		return fmt.Errorf("invalid path field %s", name)
	}
	return nil
}
