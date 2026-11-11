package endpoint

import (
	"encoding"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"reflect"

	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/internal/configuration"
	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/input"
)

// ValidationParameterIssues는 입력에서 제외한 필드를 검증기가 필수로 요구하는 충돌을 찾습니다.
// 구조 검사에 성공한 뒤 호출하며 검증 태그나 특정 검증 구현체를 강제하지 않습니다.
func ValidationParameterIssues(method reflect.Method, prefix string, validator validation.Validator) []configuration.ConfigIssue {
	return HandlerValidationParameterIssues(execution.HandlerMeta{Method: method}, prefix, validator)
}

func HandlerValidationParameterIssues(meta execution.HandlerMeta, prefix string, validator validation.Validator) []configuration.ConfigIssue {
	signature, offset := meta.FunctionType(), meta.ArgumentOffset()
	name := meta.Method.Name
	if meta.Callback.IsValid() {
		name = meta.HandlerName()
	}
	describer, ok := validator.(validation.SchemaDescriber)
	if !ok {
		return nil
	}
	var issues []configuration.ConfigIssue
	for i := offset; i < signature.NumIn(); i++ {
		parameter := signature.In(i)
		t, source, ok := input.DTO(parameter)
		if plan, wrapped, err := input.Inspect(parameter); err == nil && wrapped && plan.Source == "body" {
			t, source, ok = plan.Payload, "body", true
		}
		if !ok || !hasExcludedInputField(t, source, make(map[reflect.Type]bool)) {
			continue
		}
		declaration, err := describeValidation(describer, t)
		if err != nil {
			continue
		}
		for _, field := range declaration.Fields {
			if field.Required && excludedInputField(t, source, field.Index) {
				issues = append(issues, configuration.ConfigIssue{
					Path: fmt.Sprintf("%s.Parameters[%d]", prefix, i-offset), Code: "HTTP_INPUT_VALIDATION_CONFLICT",
					Message: fmt.Sprintf("Handler %s excludes a %s field at index %v that its validator requires.", name, source, field.Index),
					Hint:    "Bind the field or remove its required validation constraint.",
				})
			}
		}
	}
	return issues
}

func hasExcludedInputField(t reflect.Type, source string, seen map[reflect.Type]bool) bool {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || seen[t] || source == "body" && customBodyDecoder(t) {
		return false
	}
	seen[t] = true
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.IsExported() && (inputFieldExcluded(field, source) || hasExcludedInputField(field.Type, source, seen)) {
			return true
		}
	}
	return false
}

func inputFieldExcluded(field reflect.StructField, source string) bool {
	if source == "body" {
		return field.Tag.Get("json") == "-" && field.Tag.Get("xml") == "-" && field.Tag.Get("form") == "-"
	}
	return field.Tag.Get(source) == "-"
}

func customBodyDecoder(t reflect.Type) bool {
	pointer := reflect.PointerTo(t)
	return pointer.Implements(reflect.TypeFor[json.Unmarshaler]()) ||
		pointer.Implements(reflect.TypeFor[xml.Unmarshaler]()) ||
		pointer.Implements(reflect.TypeFor[encoding.TextUnmarshaler]())
}

func excludedInputField(t reflect.Type, source string, index []int) bool {
	for _, position := range index {
		if t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct || position < 0 || position >= t.NumField() {
			return false
		}
		field := t.Field(position)
		if source == "body" && customBodyDecoder(t) {
			return false
		}
		if inputFieldExcluded(field, source) {
			return true
		}
		t = field.Type
	}
	return false
}

func describeValidation(describer validation.SchemaDescriber, t reflect.Type) (result validation.SchemaDescription, err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("validation description panicked")
		}
	}()
	return describer.DescribeSchema(t)
}
