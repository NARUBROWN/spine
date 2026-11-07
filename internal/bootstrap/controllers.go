package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/internal/apiregistry"
	"github.com/NARUBROWN/spine/internal/container"
	"github.com/NARUBROWN/spine/internal/endpoint"
	"github.com/NARUBROWN/spine/internal/router"
	"github.com/NARUBROWN/spine/pkg/api"
	"github.com/NARUBROWN/spine/pkg/boot"
)

var ErrRoutesNotCollected = errors.New("controller routes require a prepared application; use DescribePreparedHTTP or static CLI export without executing constructors")

func controllerTypes(constructors []any) []reflect.Type {
	var result []reflect.Type
	seen := map[reflect.Type]bool{}
	contract := reflect.TypeFor[api.Controller]()
	for _, constructor := range constructors {
		t := reflect.TypeOf(constructor)
		if t == nil || t.Kind() != reflect.Func || t.NumOut() != 1 {
			continue
		}
		output := t.Out(0)
		if output.Kind() != reflect.Interface && output.Implements(contract) && !seen[output] {
			result = append(result, output)
			seen[output] = true
		}
	}
	return result
}

func collectControllers(ctx context.Context, config *Config, container *container.Container) error {
	if config.HTTP == nil {
		return nil
	}
	for _, owner := range controllerTypes(config.controllerCandidates()) {
		if err := ctx.Err(); err != nil {
			return err
		}
		value, err := container.Resolve(owner)
		if err != nil {
			return fmt.Errorf("controller %s: %w", owner, err)
		}
		if isNilHTTPValue(value) {
			return fmt.Errorf("controller %s is nil", owner)
		}
		registry := apiregistry.New(owner)
		if err := registerController(value.(api.Controller), registry); err != nil {
			return fmt.Errorf("controller %s RegisterRoutes: %w", owner, err)
		}
		routes, err := registry.Freeze()
		if err != nil {
			return fmt.Errorf("controller %s RegisterRoutes: %w", owner, err)
		}
		config.Routes = append(config.Routes, routes...)
	}
	config.RoutesCollected = true
	return ctx.Err()
}

func registerController(controller api.Controller, registry *api.Registry) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("registration panicked: %v", recovered)
		}
	}()
	controller.RegisterRoutes(registry)
	return nil
}

func validateCollectedRoutes(config Config, resources []boot.OpenAPIResource) error {
	issues := openAPICollisionIssues(config, resources)
	paths := map[string][]string{}
	for index, spec := range config.Routes {
		meta, err := router.Meta(spec)
		if err != nil {
			return fmt.Errorf("route %s %s: %w", spec.Method, spec.Path, err)
		}
		prefix := fmt.Sprintf("Routes[%d]", index)
		issues = append(issues, endpoint.HandlerDTOParameterIssues(meta, prefix, spec.Path)...)
		issues = append(issues, endpoint.HandlerValidationParameterIssues(meta, prefix, config.Validator)...)
		if meta.Callback.IsValid() {
			issues = append(issues, endpoint.CallableIssues(meta, prefix)...)
		}
		path, err := joinPath(config.HTTP.GlobalPrefix, spec.Path)
		if err != nil {
			return err
		}
		if err := assertNoAmbiguousRoute(spec.Method, path, paths[spec.Method]); err != nil {
			return err
		}
		paths[spec.Method] = append(paths[spec.Method], path)
		for i, interceptor := range spec.Interceptors {
			if reflect.TypeOf(interceptor) == nil {
				return fmt.Errorf("%s.Interceptors[%d] is nil", prefix, i)
			}
		}
	}
	return boot.ValidationError(issues...)
}

func controllerProviderIssues(config Config) []boot.ConfigIssue {
	if config.HTTP == nil {
		return nil
	}
	var issues []boot.ConfigIssue
	candidates, field := config.controllerCandidates(), "Constructors"
	if config.ControllerConstructors != nil {
		field = "ControllerConstructors"
	}
	for index, constructor := range candidates {
		t := reflect.TypeOf(constructor)
		if t == nil || t.Kind() != reflect.Func || t.NumOut() != 1 {
			continue
		}
		output := t.Out(0)
		if output.Kind() == reflect.Interface && output.Implements(reflect.TypeFor[api.Controller]()) {
			issues = append(issues, boot.ConfigIssue{Path: fmt.Sprintf("%s[%d]", field, index), Code: "HTTP_CONTROLLER_PROVIDER_ABSTRACT", Message: "HTTP controller constructors must return a concrete controller type.", Hint: "Return *UserController rather than api.Controller so DI can distinguish controllers without executing providers."})
		}
	}
	return issues
}

// controllerCandidates는 명시적 역할 등록을 사용한 앱의 HTTP 등록 의도를 보존합니다.
func (config Config) controllerCandidates() []any {
	if config.ControllerConstructors != nil {
		return config.ControllerConstructors
	}
	return config.Constructors
}

// componentValidationIssues는 명세 조회에서도 실행 없이 공통 생성 계약을 검사합니다.
func componentValidationIssues(config Config) []boot.ConfigIssue {
	issues := append([]boot.ConfigIssue{}, config.ComponentIssues...)
	if config.TypedComponents {
		if err := container.ValidateConstructors(config.Constructors); err != nil {
			issues = append(issues, boot.ConfigIssue{Path: "Constructors", Code: "COMPONENT_DEPENDENCIES_INVALID", Message: err.Error(), Hint: "Register one provider per type and resolve missing or circular dependencies before initialization."})
		}
	}
	return issues
}
