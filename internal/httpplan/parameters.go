package httpplan

import (
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/internal/input"
	"github.com/NARUBROWN/spine/internal/resolver"
	"github.com/NARUBROWN/spine/pkg/boot"
)

// DTOParameterIssues는 한 핸들러 안에서 같은 입력 DTO를 중복 주입하지 않도록 합니다.
// 실제 실행과 같은 리졸버 순서로 DTO를 식별하며 생성자나 요청 처리는 실행하지 않습니다.
func DTOParameterIssues(method reflect.Method, prefix string) []boot.ConfigIssue {
	seen := map[reflect.Type]int{}
	keys := map[string]int{}
	bodyIndex := -1
	var issues []boot.ConfigIssue
	resolvers := Resolvers(nil)
	for index := 1; index < method.Type.NumIn(); index++ {
		parameter := resolver.ParameterMeta{Index: index - 1, Type: method.Type.In(index)}
		var fields []input.Field
		var source string
		if payload, name, ok := input.DTO(parameter.Type); ok {
			source = name
			var err error
			fields, err = input.Fields(payload, source)
			if parameter.Type.Kind() != reflect.Pointer {
				err = fmt.Errorf("%s DTO must be a pointer to a struct", source)
			}
			if err != nil {
				issues = append(issues, boot.ConfigIssue{Path: fmt.Sprintf("%s.Parameters[%d]", prefix, parameter.Index), Code: "HTTP_INPUT_CONTRACT_INVALID", Message: err.Error(), Hint: "Use fields from one input source and mark excluded fields with '-'."})
				continue
			}
		}
		for _, candidate := range resolvers {
			if !candidate.Supports(parameter) {
				continue
			}
			switch candidate.(type) {
			case *resolver.PaginationResolver:
				source = "query"
				fields = []input.Field{{Name: "page"}, {Name: "size"}}
			case *resolver.DTOResolver, *resolver.FormDTOResolver, *resolver.InputDTOResolver:
				typ := parameter.Type.Elem()
				if first, duplicate := seen[typ]; duplicate {
					name := typ.String()
					if typ.Name() != "" {
						name = typ.PkgPath() + "." + typ.Name()
					}
					issues = append(issues, boot.ConfigIssue{
						Path:    fmt.Sprintf("%s.Parameters[%d]", prefix, parameter.Index),
						Code:    "HTTP_INPUT_DTO_DUPLICATED",
						Message: fmt.Sprintf("Handler %s repeats DTO %s at parameter indexes %d and %d.", method.Name, name, first, parameter.Index),
						Hint:    "Remove the repeated DTO parameter and read the existing argument. Parameter indexes exclude the receiver and start at zero.",
					})
					fields = nil
					break
				} else {
					seen[typ] = parameter.Index
				}
				if source == "" {
					if err := input.CheckBody(typ); err != nil {
						issues = append(issues, boot.ConfigIssue{Path: fmt.Sprintf("%s.Parameters[%d]", prefix, parameter.Index), Code: "HTTP_INPUT_CONTRACT_INVALID", Message: err.Error(), Hint: "Separate body, query and header DTOs and correct input tag names."})
					}
					if bodyIndex >= 0 {
						issues = append(issues, boot.ConfigIssue{Path: fmt.Sprintf("%s.Parameters[%d]", prefix, parameter.Index), Code: "HTTP_BODY_PARAMETERS_DUPLICATED", Message: fmt.Sprintf("Handler %s declares body parameters at indexes %d and %d.", method.Name, bodyIndex, parameter.Index), Hint: "Use at most one body DTO, including JSON and form inputs."})
					} else {
						bodyIndex = parameter.Index
					}
				}
			}
			for _, field := range fields {
				key := source + "\x00" + field.Name
				if first, exists := keys[key]; exists {
					issues = append(issues, boot.ConfigIssue{Path: fmt.Sprintf("%s.Parameters[%d]", prefix, parameter.Index), Code: "HTTP_INPUT_KEY_DUPLICATED", Message: fmt.Sprintf("Handler %s repeats %s key %q at parameter indexes %d and %d.", method.Name, source, field.Name, first, parameter.Index), Hint: "Declare each input key once within its source."})
				} else {
					keys[key] = parameter.Index
				}
			}
			break
		}
	}
	return issues
}
