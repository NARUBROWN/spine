package pipeline

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/container"
	"github.com/NARUBROWN/spine/internal/invoker"
)

type errorReturnTestError struct{ message string }

func (e *errorReturnTestError) Error() string { return e.message }

type errorReturnTestController struct{ first, second error }

func (c *errorReturnTestController) Handle() (string, error, error) {
	return "ignored", c.first, c.second
}

func TestExecute_ErrorReturnPreservesSelectedError(t *testing.T) {
	first := &errorReturnTestError{message: "first"}
	second := &errorReturnTestError{message: "second"}
	var typedNil *errorReturnTestError
	wrapped := fmt.Errorf("wrapped: %w", first)
	for _, tt := range []struct {
		name                string
		first, second, want error
		cause               error
	}{
		{name: "first_error", first: first, second: second, want: first},
		{name: "typed_nil_then_error", first: typedNil, second: second, want: second},
		{name: "nil_then_error", second: second, want: second},
		{name: "wrapped_error", first: wrapped, second: second, want: wrapped, cause: first},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctr := container.New()
			if err := ctr.RegisterConstructor(func() *errorReturnTestController {
				return &errorReturnTestController{first: tt.first, second: tt.second}
			}); err != nil {
				t.Fatal(err)
			}
			controllerType := reflect.TypeFor[*errorReturnTestController]()
			method, ok := controllerType.MethodByName("Handle")
			if !ok {
				t.Fatal("오류 반환 테스트의 핸들러가 없습니다")
			}
			p := NewPipeline(&testRouter{meta: core.HandlerMeta{
				ControllerType: controllerType,
				Method:         method,
			}}, invoker.NewInvoker(ctr))
			var handled error
			p.AddReturnValueHandler(&testReturnHandler{
				supports: func(rt reflect.Type) bool { return rt == reflect.TypeOf(tt.want) },
				handle: func(value any, _ core.ExecutionContext) error {
					handled = value.(error)
					return nil
				},
			})
			got := p.Execute(newTestExecutionContext())
			if !errors.Is(got, tt.want) || handled != tt.want {
				t.Fatalf("첫 유효 오류와 구체 타입을 보존해야 합니다: got=%v handled=%v want=%v", got, handled, tt.want)
			}
			if tt.cause != nil && !errors.Is(got, tt.cause) {
				t.Fatalf("래핑된 오류의 원인을 보존해야 합니다: %v", got)
			}
		})
	}
}
