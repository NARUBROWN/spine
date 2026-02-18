package container

import (
	"reflect"
	"strings"
	"testing"
)

type testRepo struct{}

type testService struct {
	repo *testRepo
}

type testHandler struct {
	svc *testService
}

type testIface interface {
	Name() string
}

type testImpl struct{}

func (t *testImpl) Name() string { return "impl" }

type cycleA struct {
	b *cycleB
}

type cycleB struct {
	a *cycleA
}

func TestRegisterConstructor_Validation(t *testing.T) {
	c := New()

	if err := c.RegisterConstructor(123); err == nil {
		t.Fatal("함수가 아닌 생성자에 대해 에러가 발생해야 합니다")
	}

	if err := c.RegisterConstructor(func() (*testRepo, *testService) { return nil, nil }); err == nil {
		t.Fatal("반환값이 여러 개인 생성자에 대해 에러가 발생해야 합니다")
	}
}

func TestResolve_ResolvesDependenciesAndCachesInstance(t *testing.T) {
	c := New()

	repoCalls := 0
	svcCalls := 0
	handlerCalls := 0

	_ = c.RegisterConstructor(func() *testRepo {
		repoCalls++
		return &testRepo{}
	})
	_ = c.RegisterConstructor(func(r *testRepo) *testService {
		svcCalls++
		return &testService{repo: r}
	})
	_ = c.RegisterConstructor(func(s *testService) *testHandler {
		handlerCalls++
		return &testHandler{svc: s}
	})

	typeOfHandler := reflect.TypeOf(&testHandler{})
	first, err := c.Resolve(typeOfHandler)
	if err != nil {
		t.Fatalf("Resolve에 실패했습니다: %v", err)
	}
	second, err := c.Resolve(typeOfHandler)
	if err != nil {
		t.Fatalf("두 번째 Resolve에 실패했습니다: %v", err)
	}

	if first != second {
		t.Fatal("캐시된 싱글톤 인스턴스가 반환되어야 합니다")
	}
	if repoCalls != 1 || svcCalls != 1 || handlerCalls != 1 {
		t.Fatalf("생성자는 한 번씩만 호출되어야 합니다. 실제 repo=%d svc=%d handler=%d", repoCalls, svcCalls, handlerCalls)
	}
}

func TestResolve_InterfaceAssignableConstructor(t *testing.T) {
	c := New()
	_ = c.RegisterConstructor(func() *testImpl { return &testImpl{} })

	instance, err := c.Resolve(reflect.TypeOf((*testIface)(nil)).Elem())
	if err != nil {
		t.Fatalf("인터페이스 Resolve에 실패했습니다: %v", err)
	}

	iface, ok := instance.(testIface)
	if !ok {
		t.Fatalf("Resolve 결과는 인터페이스를 구현해야 합니다. 실제 타입: %T", instance)
	}
	if iface.Name() != "impl" {
		t.Fatalf("인터페이스 구현 결과가 예상과 다릅니다: %s", iface.Name())
	}
}

func TestResolve_NoConstructor(t *testing.T) {
	c := New()
	_, err := c.Resolve(reflect.TypeOf(&testRepo{}))
	if err == nil {
		t.Fatal("등록되지 않은 생성자에 대해 에러가 발생해야 합니다")
	}
	if !strings.Contains(err.Error(), "등록된 생성자가 없습니다") {
		t.Fatalf("예상하지 못한 에러입니다: %v", err)
	}
}

func TestResolve_CycleDetection(t *testing.T) {
	c := New()
	_ = c.RegisterConstructor(func(b *cycleB) *cycleA { return &cycleA{b: b} })
	_ = c.RegisterConstructor(func(a *cycleA) *cycleB { return &cycleB{a: a} })

	_, err := c.Resolve(reflect.TypeOf(&cycleA{}))
	if err == nil {
		t.Fatal("순환 의존성 감지 에러가 발생해야 합니다")
	}
	if !strings.Contains(err.Error(), "순환 의존성 감지") {
		t.Fatalf("예상하지 못한 에러입니다: %v", err)
	}
}

func TestWarmUp_DeduplicatesAndInitializesTypes(t *testing.T) {
	c := New()

	handlerCalls := 0
	_ = c.RegisterConstructor(func() *testRepo { return &testRepo{} })
	_ = c.RegisterConstructor(func(r *testRepo) *testService { return &testService{repo: r} })
	_ = c.RegisterConstructor(func(s *testService) *testHandler {
		handlerCalls++
		return &testHandler{svc: s}
	})

	typeOfHandler := reflect.TypeOf(&testHandler{})
	err := c.WarmUp([]reflect.Type{typeOfHandler, typeOfHandler})
	if err != nil {
		t.Fatalf("WarmUp에 실패했습니다: %v", err)
	}

	if handlerCalls != 1 {
		t.Fatalf("WarmUp은 타입별로 한 번만 초기화해야 합니다. 실제 호출 횟수: %d", handlerCalls)
	}
}
