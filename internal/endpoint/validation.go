package endpoint

import (
	"fmt"
	"reflect"
	"slices"

	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/internal/input"
	"github.com/NARUBROWN/spine/pkg/boot"
)

// ValidationParameterIssues는 입력에서 제외한 필드를 검증기가 필수로 요구하는 충돌을 찾습니다.
// 구조 검사에 성공한 뒤 호출하며 검증 태그나 특정 검증 구현체를 강제하지 않습니다.
func ValidationParameterIssues(method reflect.Method, prefix string, validator validation.Validator) []boot.ConfigIssue {
	describer, ok := validator.(validation.SchemaDescriber)
	if !ok {
		return nil
	}
	var issues []boot.ConfigIssue
	for i := 1; i < method.Type.NumIn(); i++ {
		t, source, ok := input.DTO(method.Type.In(i))
		if !ok {
			continue
		}
		var excluded [][]int
		var walk func(reflect.Type, []int)
		walk = func(current reflect.Type, prefix []int) {
			for j := 0; j < current.NumField(); j++ {
				f := current.Field(j)
				if !f.IsExported() {
					continue
				}
				index := append(append([]int(nil), prefix...), j)
				if f.Tag.Get(source) == "-" {
					excluded = append(excluded, index)
				} else if f.Anonymous && f.Type.Kind() == reflect.Struct && f.Tag.Get(source) == "" {
					walk(f.Type, index)
				}
			}
		}
		walk(t, nil)
		if len(excluded) == 0 {
			continue
		}
		declaration, err := describeValidation(describer, t)
		if err != nil {
			continue
		}
		for _, field := range declaration.Fields {
			if !field.Required {
				continue
			}
			for _, index := range excluded {
				if len(field.Index) >= len(index) && slices.Equal(field.Index[:len(index)], index) {
					issues = append(issues, boot.ConfigIssue{
						Path: fmt.Sprintf("%s.Parameters[%d]", prefix, i-1), Code: "HTTP_INPUT_VALIDATION_CONFLICT",
						Message: fmt.Sprintf("Handler %s excludes a %s field at index %v that its validator requires.", method.Name, source, field.Index),
						Hint:    "Bind the field or remove its required validation constraint.",
					})
					break
				}
			}
		}
	}
	return issues
}

func describeValidation(describer validation.SchemaDescriber, t reflect.Type) (result validation.SchemaDescription, err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("validation description panicked")
		}
	}()
	return describer.DescribeSchema(t)
}
