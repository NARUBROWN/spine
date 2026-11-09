package container

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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

type otherTestImpl struct{}

func (t *otherTestImpl) Name() string { return "other" }

type cycleA struct {
	b *cycleB
}

type cycleB struct {
	a *cycleA
}

type concurrentCycleA struct {
	b *concurrentCycleB
}

type concurrentCycleB struct {
	a *concurrentCycleA
}

type cycleBarrierA struct{}

type cycleBarrierB struct{}

func TestRegisterConstructor_Validation(t *testing.T) {
	c := New()
	if err := c.RegisterConstructor(nil); err == nil || !strings.Contains(err.Error(), "must not be nil") {
		t.Fatalf("nil constructor는 명시적 설정 오류여야 합니다: %v", err)
	}

	var nilConstructor func() *testRepo
	if err := c.RegisterConstructor(nilConstructor); err == nil || !strings.Contains(err.Error(), "must not be nil") {
		t.Fatalf("typed-nil constructor는 명시적 설정 오류여야 합니다: %v", err)
	}

	if err := c.RegisterConstructor(123); err == nil {
		t.Fatal("함수가 아닌 생성자에 대해 에러가 발생해야 합니다")
	}

	if err := c.RegisterConstructor(func() (*testRepo, *testService) { return nil, nil }); err == nil {
		t.Fatal("반환값이 여러 개인 생성자에 대해 에러가 발생해야 합니다")
	}
}

func TestResolve_TypedNilConstructorResultReturnsError(t *testing.T) {
	c := New()
	if err := c.RegisterConstructor(func() *testRepo { return nil }); err != nil {
		t.Fatalf("생성자 등록 실패: %v", err)
	}

	instance, err := c.Resolve(reflect.TypeOf(&testRepo{}))
	if err == nil || !strings.Contains(err.Error(), "returned nil") {
		t.Fatalf("typed-nil 결과는 명시적 Resolve 오류여야 합니다: instance=%v err=%v", instance, err)
	}
}

func TestResolve_AllowsNilCollectionConstructorResults(t *testing.T) {
	type collectionConsumer struct {
		values []string
	}

	c := New()
	if err := c.RegisterConstructor(func() []string { return nil }); err != nil {
		t.Fatalf("nil slice 생성자 등록 실패: %v", err)
	}
	if err := c.RegisterConstructor(func(values []string) *collectionConsumer {
		return &collectionConsumer{values: values}
	}); err != nil {
		t.Fatalf("collection consumer 등록 실패: %v", err)
	}

	resolved, err := c.Resolve(reflect.TypeFor[*collectionConsumer]())
	if err != nil {
		t.Fatalf("nil collection provider를 의존성으로 사용할 수 있어야 합니다: %v", err)
	}
	consumer := resolved.(*collectionConsumer)
	if consumer.values != nil {
		t.Fatalf("nil slice 값이 보존되어야 합니다: %#v", consumer.values)
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

func TestResolve_InterfaceAndConcreteShareCanonicalSingleton(t *testing.T) {
	for _, interfaceFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "concrete-first", true: "interface-first"}[interfaceFirst], func(t *testing.T) {
			c := New()
			calls := 0
			if err := c.RegisterConstructor(func() *testImpl {
				calls++
				return &testImpl{}
			}); err != nil {
				t.Fatalf("생성자 등록 실패: %v", err)
			}

			concreteType := reflect.TypeOf(&testImpl{})
			interfaceType := reflect.TypeOf((*testIface)(nil)).Elem()
			firstType, secondType := concreteType, interfaceType
			if interfaceFirst {
				firstType, secondType = interfaceType, concreteType
			}

			first, err := c.Resolve(firstType)
			if err != nil {
				t.Fatalf("첫 Resolve 실패: %v", err)
			}
			second, err := c.Resolve(secondType)
			if err != nil {
				t.Fatalf("두 번째 Resolve 실패: %v", err)
			}
			if first != second || calls != 1 {
				t.Fatalf("interface/concrete는 같은 singleton이어야 합니다: first=%p second=%p calls=%d", first, second, calls)
			}
		})
	}
}

func TestResolve_ConcurrentInterfaceAndConcreteBuildOnce(t *testing.T) {
	c := New()
	release := make(chan struct{})
	started := make(chan struct{}, 2)
	var calls atomic.Int32
	if err := c.RegisterConstructor(func() *testImpl {
		calls.Add(1)
		started <- struct{}{}
		<-release
		return &testImpl{}
	}); err != nil {
		t.Fatalf("생성자 등록 실패: %v", err)
	}

	types := []reflect.Type{
		reflect.TypeOf(&testImpl{}),
		reflect.TypeOf((*testIface)(nil)).Elem(),
	}
	results := make(chan any, len(types))
	errs := make(chan error, len(types))
	launch := make(chan struct{})
	for _, target := range types {
		go func() {
			<-launch
			instance, err := c.Resolve(target)
			results <- instance
			errs <- err
		}()
	}
	close(launch)
	<-started
	close(release)

	first, second := <-results, <-results
	for range types {
		if err := <-errs; err != nil {
			t.Fatalf("동시 interface/concrete Resolve 실패: %v", err)
		}
	}
	if calls.Load() != 1 || first != second {
		t.Fatalf("동시 alias Resolve도 canonical build 하나를 공유해야 합니다: calls=%d first=%p second=%p", calls.Load(), first, second)
	}
}

func TestResolve_NoConstructor(t *testing.T) {
	c := New()
	_, err := c.Resolve(reflect.TypeOf(&testRepo{}))
	if err == nil {
		t.Fatal("등록되지 않은 생성자에 대해 에러가 발생해야 합니다")
	}
	if !strings.Contains(err.Error(), "no constructor registered") {
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
	if !strings.Contains(err.Error(), "circular dependency detected") {
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

func TestResolve_ConcurrentResolveBuildsSingletonOnce(t *testing.T) {
	c := New()

	start := make(chan struct{})
	var constructorCalls atomic.Int32

	if err := c.RegisterConstructor(func() *testRepo {
		constructorCalls.Add(1)
		<-start
		return &testRepo{}
	}); err != nil {
		t.Fatalf("생성자 등록 실패: %v", err)
	}

	typeOfRepo := reflect.TypeOf(&testRepo{})
	results := make([]any, 2)
	errs := make([]error, 2)

	var wg sync.WaitGroup
	for i := range 2 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = c.Resolve(typeOfRepo)
		}(i)
	}

	close(start)
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			t.Fatalf("동시 Resolve 실패: %v", err)
		}
	}
	if constructorCalls.Load() != 1 {
		t.Fatalf("생성자는 한 번만 호출되어야 합니다. 실제=%d", constructorCalls.Load())
	}
	if results[0] != results[1] {
		t.Fatal("동시 Resolve도 동일 싱글톤을 반환해야 합니다")
	}
}

func TestResolve_InterfaceWithMultipleImplementationsReturnsError(t *testing.T) {
	c := New()
	_ = c.RegisterConstructor(func() *testImpl { return &testImpl{} })
	_ = c.RegisterConstructor(func() *otherTestImpl { return &otherTestImpl{} })

	_, err := c.Resolve(reflect.TypeOf((*testIface)(nil)).Elem())
	if err == nil {
		t.Fatal("구현체가 여러 개면 에러가 발생해야 합니다")
	}
	if !strings.Contains(err.Error(), "multiple constructors registered") {
		t.Fatalf("예상하지 못한 에러입니다: %v", err)
	}
}

func TestResolve_ConcurrentConstructorPanicsAreReturnedWithoutLeakingPanic(t *testing.T) {
	c := New()

	panicErr := errors.New("constructor panic")
	if err := c.RegisterConstructor(func() *testRepo {
		panic(panicErr)
	}); err != nil {
		t.Fatalf("생성자 등록 실패: %v", err)
	}

	type result struct {
		instance any
		err      error
		panicVal any
	}
	resolve := func(out chan<- result) {
		res := result{}
		defer func() {
			res.panicVal = recover()
			out <- res
		}()
		res.instance, res.err = c.Resolve(reflect.TypeOf(&testRepo{}))
	}

	const resolvers = 32
	results := make(chan result, resolvers)
	launch := make(chan struct{})
	for range resolvers {
		go func() {
			<-launch
			resolve(results)
		}()
	}
	close(launch)

	for range resolvers {
		select {
		case res := <-results:
			if res.panicVal != nil {
				t.Fatalf("생성자 panic은 Resolve 오류로 변환되어야 합니다: %v", res.panicVal)
			}
			if res.instance != nil {
				t.Fatalf("생성 실패 시 인스턴스는 nil이어야 합니다: %T", res.instance)
			}
			if !errors.Is(res.err, panicErr) {
				t.Fatalf("모든 동시 호출에 생성자 panic 원인이 전달되어야 합니다: %v", res.err)
			}
		case <-time.After(time.Second):
			t.Fatal("생성자 panic 이후 Resolve가 종료되지 않았습니다")
		}
	}
}

func TestResolve_ConcurrentCycleReturnsErrorWithoutDeadlock(t *testing.T) {
	c := New()

	aStarted := make(chan struct{})
	bStarted := make(chan struct{})
	_ = c.RegisterConstructor(func() *cycleBarrierA {
		close(aStarted)
		<-bStarted
		return &cycleBarrierA{}
	})
	_ = c.RegisterConstructor(func() *cycleBarrierB {
		close(bStarted)
		<-aStarted
		return &cycleBarrierB{}
	})
	_ = c.RegisterConstructor(func(_ *cycleBarrierA, b *concurrentCycleB) *concurrentCycleA {
		return &concurrentCycleA{b: b}
	})
	_ = c.RegisterConstructor(func(_ *cycleBarrierB, a *concurrentCycleA) *concurrentCycleB {
		return &concurrentCycleB{a: a}
	})

	errs := make(chan error, 2)
	go func() {
		_, err := c.Resolve(reflect.TypeOf(&concurrentCycleA{}))
		errs <- err
	}()
	go func() {
		_, err := c.Resolve(reflect.TypeOf(&concurrentCycleB{}))
		errs <- err
	}()

	for range 2 {
		select {
		case err := <-errs:
			if err == nil || !strings.Contains(err.Error(), "circular dependency detected") {
				t.Fatalf("동시 순환 의존성은 명시적 오류여야 합니다: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("동시 순환 의존성 해석이 교착 상태에 빠졌습니다")
		}
	}
}
