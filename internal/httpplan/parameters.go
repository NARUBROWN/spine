package httpplan

import (
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/internal/resolver"
	"github.com/NARUBROWN/spine/pkg/boot"
)

// DTOParameterIssues는 한 핸들러 안에서 같은 입력 DTO를 중복 주입하지 않도록 합니다.
// 실제 실행과 같은 리졸버 순서로 DTO를 식별하며 생성자나 요청 처리는 실행하지 않습니다.
func DTOParameterIssues(method reflect.Method, prefix string) []boot.ConfigIssue {
	seen := map[reflect.Type]int{}
	var issues []boot.ConfigIssue
	resolvers := Resolvers(nil)
	for index := 1; index < method.Type.NumIn(); index++ {
		parameter := resolver.ParameterMeta{Index: index - 1, Type: method.Type.In(index)}
		for _, candidate := range resolvers {
			if !candidate.Supports(parameter) {
				continue
			}
			switch candidate.(type) {
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
				} else {
					seen[typ] = parameter.Index
				}
			}
			break
		}
	}
	return issues
}
