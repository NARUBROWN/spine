package spine

import (
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/container"
	"github.com/NARUBROWN/spine/pkg/api"
	"github.com/NARUBROWN/spine/pkg/boot"
)

// ControllerConstructor는 생성 계약과 HTTP 컨트롤러 계약을 함께 등록합니다.
func (a *app) ControllerConstructor(prototypes ...any) {
	a.componentConstructors("Controller", reflect.TypeFor[api.Controller](), prototypes)
}

// ServiceConstructor는 생성 계약을 따르는 서비스를 등록합니다.
func (a *app) ServiceConstructor(prototypes ...any) {
	a.componentConstructors("Service", nil, prototypes)
}

// RepositoryConstructor는 생성 계약을 따르는 저장소를 등록합니다.
func (a *app) RepositoryConstructor(prototypes ...any) {
	a.componentConstructors("Repository", nil, prototypes)
}

// InterceptorConstructor는 생성 계약과 인터셉터 계약을 등록합니다.
// 실행 범위와 순서는 Interceptor/InterceptorFor 또는 경로 옵션으로 지정합니다.
func (a *app) InterceptorConstructor(prototypes ...any) {
	a.componentConstructors("Interceptor", reflect.TypeFor[core.Interceptor](), prototypes)
}

func (a *app) componentConstructors(role string, contract reflect.Type, prototypes []any) {
	if !a.typedComponents {
		a.controllerConstructors = append([]any{}, a.constructors...)
	}
	a.typedComponents = true
	if a.componentCounts == nil {
		a.componentCounts = map[string]int{}
	}
	for _, prototype := range prototypes {
		index := a.componentCounts[role]
		a.componentCounts[role]++
		path := fmt.Sprintf("%sConstructors[%d]", role, index)
		constructor, err := container.ObjectConstructor(prototype)
		if err != nil {
			a.componentIssues = append(a.componentIssues, boot.ConfigIssue{Path: path, Code: "COMPONENT_CONSTRUCTOR_INVALID", Message: err.Error(), Hint: "Implement Constructor(dependencies...) returning a pointer to the same concrete type."})
			continue
		}
		owner := reflect.TypeOf(prototype)
		if contract != nil && !owner.Implements(contract) {
			hint := "Implement RegisterRoutes(*api.Registry) in addition to the Constructor method."
			if role == "Interceptor" {
				hint = "Implement PreHandle, PostHandle, BeforeResponse and AfterCompletion in addition to the Constructor method."
			}
			a.componentIssues = append(a.componentIssues, boot.ConfigIssue{Path: path, Code: "COMPONENT_CONTRACT_INVALID", Message: fmt.Sprintf("%v must implement %v", owner, contract), Hint: hint})
			continue
		}
		a.constructors = append(a.constructors, constructor)
		if role == "Controller" {
			a.controllerConstructors = append(a.controllerConstructors, constructor)
		}
	}
}
