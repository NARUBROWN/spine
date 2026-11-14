package spine

import (
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/componentregistration"
	"github.com/NARUBROWN/spine/internal/container"
	"github.com/NARUBROWN/spine/pkg/api"
	"github.com/NARUBROWN/spine/pkg/boot"
)

func (a *app) componentRegistration(registration componentregistration.Registration) {
	var contract reflect.Type
	role, value := componentregistration.Unpack(registration)
	switch role {
	case "Custom", "Repository", "Service":
	case "Controller":
		contract = reflect.TypeFor[api.Controller]()
	case "Interceptor":
		contract = reflect.TypeFor[core.Interceptor]()
	default:
		a.componentIssues = append(a.componentIssues, boot.ConfigIssue{Path: "Constructors", Code: "COMPONENT_CONSTRUCTOR_INVALID", Message: "component registration must declare a role", Hint: "Use constructor.Custom, Repository, Service, Controller or Interceptor."})
		return
	}
	a.componentConstructors(role, contract, []any{value})
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
		if t := reflect.TypeOf(prototype); role == "Custom" && t != nil && t.Kind() == reflect.Func {
			if err := container.New().RegisterConstructor(prototype); err != nil {
				a.componentIssues = append(a.componentIssues, boot.ConfigIssue{Path: path, Code: "COMPONENT_CONSTRUCTOR_INVALID", Message: err.Error(), Hint: "Provide a non-nil constructor function returning a value or a value and error."})
				continue
			}
			a.constructors = append(a.constructors, prototype)
			continue
		}
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
