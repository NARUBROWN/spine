package endpoint

import (
	"encoding"
	"fmt"
	"reflect"

	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/internal/configuration"
	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/input"
	"github.com/NARUBROWN/spine/internal/resolver"
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
		if source, wrapped := input.Wrapper(parameter); wrapped && source == "body" &&
			parameter.Kind() == reflect.Pointer && parameter.Elem().Kind() == reflect.Struct {
			if !input.BodyMediaNeedsValidation(parameter.Elem()) {
				continue
			}
			declaration, err := resolver.DescribeValidation(describer, parameter.Elem())
			if err == nil && len(input.BodyMediaTypes(parameter.Elem(), declaration)) == 0 {
				issues = append(issues, configuration.ConfigIssue{
					Path: fmt.Sprintf("%s.Parameters[%d]", prefix, i-offset), Code: "HTTP_INPUT_VALIDATION_CONFLICT",
					Message: fmt.Sprintf("Handler %s has no body format that can bind all required fields.", name),
					Hint:    "Bind the required fields in a common body format or change their validation constraints.",
				})
			}
			continue
		}
		t, source, ok := input.DTO(parameter)
		if !ok || !hasExcludedInputField(t, source, make(map[reflect.Type]bool)) {
			continue
		}
		declaration, err := resolver.DescribeValidation(describer, t)
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
	if t.Kind() != reflect.Struct || seen[t] {
		return false
	}
	seen[t] = true
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.IsExported() && (field.Tag.Get(source) == "-" || hasExcludedInputField(field.Type, source, seen)) {
			return true
		}
	}
	return false
}

func excludedInputField(t reflect.Type, source string, index []int) bool {
	for depth, position := range index {
		if t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct || position < 0 || position >= t.NumField() {
			return false
		}
		field := t.Field(position)
		if depth > 0 && reflect.PointerTo(t).Implements(reflect.TypeFor[encoding.TextUnmarshaler]()) {
			return false
		}
		if field.Tag.Get(source) == "-" {
			return true
		}
		t = field.Type
	}
	return false
}
