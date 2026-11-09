package endpoint

import (
	"fmt"
	"github.com/NARUBROWN/spine/core"
	"reflect"

	"github.com/NARUBROWN/spine/internal/input"
	"github.com/NARUBROWN/spine/internal/resolver"
	"github.com/NARUBROWN/spine/pkg/boot"
)

// DTOParameterIssues는 한 핸들러 안에서 같은 입력 DTO를 중복 주입하지 않도록 합니다.
// 실제 실행과 같은 리졸버 순서로 DTO를 식별하며 생성자나 요청 처리는 실행하지 않습니다.
func DTOParameterIssues(method reflect.Method, prefix, routePath string) []boot.ConfigIssue {
	return HandlerDTOParameterIssues(core.HandlerMeta{Method: method}, prefix, routePath)
}

func HandlerDTOParameterIssues(meta core.HandlerMeta, prefix, routePath string) []boot.ConfigIssue {
	signature, offset := meta.FunctionType(), meta.ArgumentOffset()
	handlerName := meta.Method.Name
	if meta.Callback.IsValid() {
		handlerName = meta.HandlerName()
	}
	pathKeys, err := PathKeys(routePath)
	if err != nil {
		return []boot.ConfigIssue{{Path: prefix + ".Path", Code: "HTTP_PATH_TEMPLATE_INVALID", Message: err.Error(), Hint: "Use a distinct name for each path parameter."}}
	}
	declaredPathKeys := map[string]bool{}
	for _, name := range pathKeys {
		declaredPathKeys[name] = true
	}
	legacyIndex, pathDTOIndex, legacyCount := -1, -1, 0
	legacyAPIIndex, typedAPIIndex := -1, -1
	seen := map[reflect.Type]int{}
	keys := map[string]int{}
	bodyIndex := -1
	scalarPathIndex := -1
	var issues []boot.ConfigIssue
	resolvers := Resolvers(nil)
	for index := offset; index < signature.NumIn(); index++ {
		parameter := resolver.ParameterMeta{Index: index - offset, Type: signature.In(index)}
		legacyPagination := resolver.IsLegacyPagination(parameter.Type)
		if legacyPagination {
			if parameter.Type.Kind() == reflect.Pointer {
				issues = append(issues, boot.ConfigIssue{Path: fmt.Sprintf("%s.Parameters[%d]", prefix, parameter.Index), Code: "HTTP_INPUT_CONTRACT_INVALID", Message: "Deprecated query.Pagination must be a value.", Hint: "Keep the existing value parameter or migrate page and size to a query-tagged struct pointer."})
				continue
			}
			if legacyAPIIndex < 0 {
				legacyAPIIndex = parameter.Index
			}
		}
		if resolver.IsLegacyPath(parameter.Type) {
			legacyIndex, legacyCount = parameter.Index, legacyCount+1
			if legacyAPIIndex < 0 {
				legacyAPIIndex = parameter.Index
			}
			if parameter.Type.Kind() == reflect.Pointer {
				issues = append(issues, boot.ConfigIssue{Path: fmt.Sprintf("%s.Parameters[%d]", prefix, parameter.Index), Code: "HTTP_INPUT_CONTRACT_INVALID", Message: "Legacy path wrappers must be values.", Hint: "Migrate to a struct pointer with path tags."})
			}
			continue
		}
		if resolver.IsLegacyValues(parameter.Type) {
			if parameter.Type.Kind() == reflect.Pointer {
				issues = append(issues, boot.ConfigIssue{Path: fmt.Sprintf("%s.Parameters[%d]", prefix, parameter.Index), Code: "HTTP_INPUT_CONTRACT_INVALID", Message: "Deprecated query.Values and header.Values must be values.", Hint: "Use the existing value parameter or migrate to a tagged struct pointer."})
			} else if legacyAPIIndex < 0 {
				legacyAPIIndex = parameter.Index
			}
			continue
		}
		var fields []input.Field
		var source string
		plan, planned, planErr := input.Inspect(parameter.Type)
		if planErr != nil {
			issues = append(issues, boot.ConfigIssue{Path: fmt.Sprintf("%s.Parameters[%d]", prefix, parameter.Index), Code: "HTTP_INPUT_CONTRACT_INVALID", Message: planErr.Error(), Hint: "Use a struct payload with fields from one input source."})
			continue
		}
		if plan.Scalar {
			if scalarPathIndex >= 0 || len(pathKeys) != 1 {
				issues = append(issues, boot.ConfigIssue{Path: fmt.Sprintf("%s.Parameters[%d]", prefix, parameter.Index), Code: "HTTP_PATH_SCALAR_AMBIGUOUS", Message: "A scalar path input requires exactly one route key and one scalar path argument.", Hint: "Use one scalar path argument for a route with one key, or use a path DTO."})
			}
			scalarPathIndex = parameter.Index
			if len(pathKeys) == 1 {
				plan.Fields = []input.Field{{Name: pathKeys[0], Type: plan.Payload}}
			}
		}
		if planned && plan.Source != "body" {
			source = plan.Source
			fields = plan.Fields
			var err error
			if parameter.Type.Kind() != reflect.Pointer {
				err = fmt.Errorf("%s DTO must be a pointer to a struct", source)
			}
			if err != nil {
				issues = append(issues, boot.ConfigIssue{Path: fmt.Sprintf("%s.Parameters[%d]", prefix, parameter.Index), Code: "HTTP_INPUT_CONTRACT_INVALID", Message: err.Error(), Hint: "Use fields from one input source and mark excluded fields with '-'."})
				continue
			}
			if typedAPIIndex < 0 {
				typedAPIIndex = parameter.Index
			}
			if source == "path" {
				pathDTOIndex = parameter.Index
				for _, field := range fields {
					if !declaredPathKeys[field.Name] {
						issues = append(issues, boot.ConfigIssue{Path: fmt.Sprintf("%s.Parameters[%d]", prefix, parameter.Index), Code: "HTTP_PATH_KEY_UNDECLARED", Message: fmt.Sprintf("Handler %s binds path key %q that its route does not declare.", handlerName, field.Name), Hint: "Match path tags to this route's named parameters."})
					}
				}
			}
		}
		for _, candidate := range resolvers {
			if !candidate.Supports(parameter) {
				continue
			}
			switch candidate.(type) {
			case *resolver.ScalarPathResolver:
				// 스칼라와 DTO가 같은 경로 키를 중복 선언하는지 공통 검사합니다.
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
						Message: fmt.Sprintf("Handler %s repeats DTO %s at parameter indexes %d and %d.", handlerName, name, first, parameter.Index),
						Hint:    "Remove the repeated DTO parameter and read the existing argument. Parameter indexes exclude the receiver and start at zero.",
					})
					fields = nil
					break
				} else {
					seen[typ] = parameter.Index
				}
				if source == "" {
					if err := input.CheckBody(typ); err != nil {
						issues = append(issues, boot.ConfigIssue{Path: fmt.Sprintf("%s.Parameters[%d]", prefix, parameter.Index), Code: "HTTP_INPUT_CONTRACT_INVALID", Message: err.Error(), Hint: "Separate body, path, query and header DTOs and correct input tag names."})
					}
					if bodyIndex >= 0 {
						issues = append(issues, boot.ConfigIssue{Path: fmt.Sprintf("%s.Parameters[%d]", prefix, parameter.Index), Code: "HTTP_BODY_PARAMETERS_DUPLICATED", Message: fmt.Sprintf("Handler %s declares body parameters at indexes %d and %d.", handlerName, bodyIndex, parameter.Index), Hint: "Use at most one body DTO, including JSON and form inputs."})
					} else {
						bodyIndex = parameter.Index
					}
				}
			}
			for _, field := range fields {
				key := source + "\x00" + field.Name
				if legacyPagination {
					// 기존 API와 새 API의 혼용 오류와 키 중복 오류를 중복해서 반환하지 않습니다.
					key = "legacy\x00" + key
				}
				if first, exists := keys[key]; exists {
					issues = append(issues, boot.ConfigIssue{Path: fmt.Sprintf("%s.Parameters[%d]", prefix, parameter.Index), Code: "HTTP_INPUT_KEY_DUPLICATED", Message: fmt.Sprintf("Handler %s repeats %s key %q at parameter indexes %d and %d.", handlerName, source, field.Name, first, parameter.Index), Hint: "Declare each input key once within its source."})
				} else {
					keys[key] = parameter.Index
				}
			}
			break
		}
	}
	if legacyIndex >= 0 && pathDTOIndex >= 0 {
		issues = append(issues, boot.ConfigIssue{Path: fmt.Sprintf("%s.Parameters[%d]", prefix, pathDTOIndex), Code: "HTTP_PATH_INPUT_MIXED", Message: fmt.Sprintf("Handler %s mixes legacy path wrappers and path DTOs.", handlerName), Hint: "Move every path binding in this handler to path-tagged struct fields."})
	} else if legacyAPIIndex >= 0 && typedAPIIndex >= 0 {
		issues = append(issues, boot.ConfigIssue{Path: fmt.Sprintf("%s.Parameters[%d]", prefix, typedAPIIndex), Code: "HTTP_INPUT_API_MIXED", Message: fmt.Sprintf("Handler %s mixes deprecated and typed path/query/header inputs at parameter indexes %d and %d.", handlerName, legacyAPIIndex, typedAPIIndex), Hint: "Migrate every deprecated input, including query.Pagination, to tagged struct pointers. Existing body DTOs and contexts may still accompany deprecated inputs."})
	} else if legacyCount > len(pathKeys) {
		issues = append(issues, boot.ConfigIssue{Path: fmt.Sprintf("%s.Parameters[%d]", prefix, legacyIndex), Code: "HTTP_PATH_ARGUMENT_UNBOUND", Message: fmt.Sprintf("Handler %s declares more legacy path arguments than its route has keys.", handlerName), Hint: "Migrate to named path fields or remove the unbound argument."})
	}
	return issues
}
