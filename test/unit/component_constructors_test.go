package spine

import (
	"context"
	"strings"
	"testing"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/api"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/constructor"
	contextapi "github.com/NARUBROWN/spine/pkg/spine"
)

type componentDependency struct{}

func (*componentDependency) Constructor() *componentDependency { return &componentDependency{} }

type componentService struct{}

func (*componentService) Constructor(*componentDependency) *componentService {
	return &componentService{}
}

type componentController struct{}

func (*componentController) Constructor(*componentService) *componentController {
	return &componentController{}
}
func (*componentController) RegisterRoutes(*api.Registry) {}

type componentMissingController struct{}

func (*componentMissingController) Constructor() *componentMissingController { panic("must not run") }

type componentWrongRoutes struct{}

func (*componentWrongRoutes) Constructor() *componentWrongRoutes { panic("must not run") }
func (*componentWrongRoutes) RegisterRoutes(string)              {}

type componentInterceptor struct{}

func (*componentInterceptor) Constructor() *componentInterceptor               { return &componentInterceptor{} }
func (*componentInterceptor) PreHandle(contextapi.Ctx, core.HandlerMeta) error { return nil }
func (*componentInterceptor) PostHandle(contextapi.Ctx, core.HandlerMeta)      {}
func (*componentInterceptor) BeforeResponse(contextapi.Ctx, core.HandlerMeta, error) error {
	return nil
}
func (*componentInterceptor) AfterCompletion(contextapi.Ctx, core.HandlerMeta, error) {}

type componentTransport struct{ initialized *bool }

func (t *componentTransport) Init(core.Container) error { *t.initialized = true; return nil }
func (*componentTransport) Start() error                { return nil }
func (*componentTransport) Stop(context.Context) error  { return nil }

func TestRoleConstructorContracts(t *testing.T) {
	for _, tc := range []struct {
		name     string
		register func(App)
		want     string
	}{
		{"controller", func(a App) { a.Constructor(constructor.Controller(&componentMissingController{})) }, "COMPONENT_CONTRACT_INVALID"},
		{"wrong controller signature", func(a App) { a.Constructor(constructor.Controller(&componentWrongRoutes{})) }, "COMPONENT_CONTRACT_INVALID"},
		{"interceptor", func(a App) { a.Constructor(constructor.Interceptor(&componentMissingController{})) }, "COMPONENT_CONTRACT_INVALID"},
		{"constructor", func(a App) { a.Constructor(constructor.Service(&struct{}{})) }, "COMPONENT_CONSTRUCTOR_INVALID"},
		{"dependency", func(a App) { a.Constructor(constructor.Service(&componentService{})) }, "COMPONENT_DEPENDENCIES_INVALID"},
		{"empty marker", func(a App) { a.Constructor(constructor.Registration{}) }, "COMPONENT_CONSTRUCTOR_INVALID"},
		{"nil custom", func(a App) { a.Constructor(constructor.Custom(nil)) }, "COMPONENT_CONSTRUCTOR_INVALID"},
		{"nil factory", func(a App) { a.Constructor(constructor.Custom((func() *componentDependency)(nil))) }, "COMPONENT_CONSTRUCTOR_INVALID"},
		{"invalid factory", func(a App) {
			a.Constructor(constructor.Custom(func() (*componentDependency, error) { panic("must not run") }))
		}, "COMPONENT_CONSTRUCTOR_INVALID"},
		{"factory outside custom", func(a App) {
			a.Constructor(constructor.Repository(func() *componentDependency { panic("must not run") }))
		}, "COMPONENT_CONSTRUCTOR_INVALID"},
		{"custom factory graph", func(a App) {
			a.Constructor(constructor.Custom(func(*componentDependency) *componentService { panic("must not run") }))
		}, "COMPONENT_DEPENDENCIES_INVALID"},
		{"duplicate", func(a App) {
			a.Constructor(constructor.Repository(&componentDependency{}))
			a.Constructor(func() *componentDependency { return nil })
		}, "COMPONENT_DEPENDENCIES_INVALID"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := New()
			initialized := false
			a.RegisterTransport(&componentTransport{&initialized})
			tc.register(a)
			for range 2 {
				if err := a.Validate(boot.Options{}); err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("validation: %v", err)
				}
			}
			if _, err := a.Prepare(context.Background(), boot.Options{}); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("prepare: %v", err)
			}
			if initialized {
				t.Fatal("invalid components initialized a transport")
			}
			if _, err := DescribeHTTP(a, boot.Options{}); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("description: %v", err)
			}
		})
	}
}
func TestAllComponentRolesShareOneContainer(t *testing.T) {
	a := New().(*app)
	a.Constructor(
		constructor.Custom(&componentDependency{}),
		constructor.Repository(&componentService{}),
		constructor.Service(&componentService{}),
	)
	if err := a.Validate(boot.Options{}); err == nil {
		t.Fatal("duplicate cross-role providers accepted")
	}
	a = New().(*app)
	a.Constructor(
		constructor.Repository(&componentDependency{}),
		constructor.Service(&componentService{}),
		constructor.Controller(&componentController{}),
		constructor.Interceptor(&componentInterceptor{}),
	)
	if err := a.Validate(boot.Options{}); err != nil {
		t.Fatal(err)
	}
	if len(a.interceptors) != 0 {
		t.Fatal("registration applied an interceptor")
	}
	for range 2 {
		p, err := a.Prepare(context.Background(), boot.Options{})
		if err != nil {
			t.Fatal(err)
		}
		if err = p.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
}
