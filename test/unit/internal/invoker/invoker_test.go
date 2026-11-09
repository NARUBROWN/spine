package invoker

import (
	"reflect"
	"strings"
	"testing"

	"github.com/NARUBROWN/spine/internal/container"
)

type testController struct{}

type namedEvent string
type namedPayload []byte

type extendedControllerContext interface {
	Get(key string) (any, bool)
	Principal() string
}

func (c *testController) Echo(v int) (int, string) {
	return v, "ok"
}

func (c *testController) Named(event namedEvent, payload namedPayload) string {
	return string(event) + ":" + string(payload)
}

func (c *testController) ExtendedContext(ctx extendedControllerContext) string {
	return ctx.Principal()
}

type baseControllerContext struct{}

func (baseControllerContext) Get(string) (any, bool) { return nil, false }

func TestInvoker_InvokeReturnsMethodResults(t *testing.T) {
	ctr := container.New()
	if err := ctr.RegisterConstructor(func() *testController { return &testController{} }); err != nil {
		t.Fatalf("생성자 등록 실패: %v", err)
	}

	controllerType := reflect.TypeOf(&testController{})
	method, ok := controllerType.MethodByName("Echo")
	if !ok {
		t.Fatal("Echo 메서드를 찾을 수 없습니다")
	}

	results, err := NewInvoker(ctr).Invoke(controllerType, method, []any{7})
	if err != nil {
		t.Fatalf("Invoke 실패: %v", err)
	}
	if len(results) != 2 || results[0].(int) != 7 || results[1].(string) != "ok" {
		t.Fatalf("반환값이 잘못되었습니다: %#v", results)
	}
}

func TestInvoker_InvokeReturnsResolveError(t *testing.T) {
	ctr := container.New()
	controllerType := reflect.TypeOf(&testController{})
	method, ok := controllerType.MethodByName("Echo")
	if !ok {
		t.Fatal("Echo 메서드를 찾을 수 없습니다")
	}

	_, err := NewInvoker(ctr).Invoke(controllerType, method, []any{7})
	if err == nil {
		t.Fatal("Resolve 실패는 그대로 반환되어야 합니다")
	}
}

func TestInvoker_ConvertsNamedStringAndByteSliceArguments(t *testing.T) {
	ctr := container.New()
	if err := ctr.RegisterConstructor(func() *testController { return &testController{} }); err != nil {
		t.Fatalf("생성자 등록 실패: %v", err)
	}
	controllerType := reflect.TypeOf(&testController{})
	method, _ := controllerType.MethodByName("Named")

	results, err := NewInvoker(ctr).Invoke(controllerType, method, []any{"order.created", []byte("body")})
	if err != nil {
		t.Fatalf("convertible resolver 값은 named parameter로 안전하게 변환되어야 합니다: %v", err)
	}
	if got := results[0].(string); got != "order.created:body" {
		t.Fatalf("named parameter 결과가 잘못되었습니다: %s", got)
	}
}

func TestInvoker_IncompatibleExtendedControllerContextReturnsError(t *testing.T) {
	ctr := container.New()
	if err := ctr.RegisterConstructor(func() *testController { return &testController{} }); err != nil {
		t.Fatalf("생성자 등록 실패: %v", err)
	}
	controllerType := reflect.TypeOf(&testController{})
	method, _ := controllerType.MethodByName("ExtendedContext")

	_, err := NewInvoker(ctr).Invoke(controllerType, method, []any{baseControllerContext{}})
	if err == nil || !strings.Contains(err.Error(), "not assignable") {
		t.Fatalf("확장 ControllerContext 불일치는 reflection panic 대신 오류여야 합니다: %v", err)
	}
}
