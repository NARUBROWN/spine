package invoker

import (
	"reflect"
	"testing"

	"github.com/NARUBROWN/spine/internal/container"
)

type testController struct{}

func (c *testController) Echo(v int) (int, string) {
	return v, "ok"
}

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
