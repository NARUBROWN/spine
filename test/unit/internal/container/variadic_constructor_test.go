package container

import (
	"reflect"
	"strings"
	"testing"
)

func TestRegisterConstructorRejectsVariadicWithoutReplacingProvider(t *testing.T) {
	for _, withFixedDependency := range []bool{false, true} {
		t.Run(map[bool]string{false: "variadic-only", true: "fixed-and-variadic"}[withFixedDependency], func(t *testing.T) {
			c := New()
			original := &testService{}
			originalCalls, variadicCalls := 0, 0
			if err := c.RegisterConstructor(func() *testService {
				originalCalls++
				return original
			}); err != nil {
				t.Fatal(err)
			}
			var invalid any = func(...string) *testService {
				variadicCalls++
				return nil
			}
			if withFixedDependency {
				invalid = func(*testRepo, ...string) *testService {
					variadicCalls++
					return nil
				}
			}
			err := c.RegisterConstructor(invalid)
			if err == nil || !strings.Contains(err.Error(), "must be non-variadic") {
				t.Fatalf("가변 인자 생성자를 등록 시 거부해야 합니다: %v", err)
			}
			value, err := c.Resolve(reflect.TypeFor[*testService]())
			if err != nil || value != original || originalCalls != 1 || variadicCalls != 0 {
				t.Fatalf("거부된 생성자가 기존 등록을 바꾸면 안 됩니다: value=%v err=%v calls=%d/%d", value, err, originalCalls, variadicCalls)
			}
		})
	}
}

func TestValidateConstructorsRejectsVariadicBeforeExecutingProviders(t *testing.T) {
	calls := 0
	sliceProvider := func() []string {
		calls++
		return []string{"one", "two"}
	}
	variadic := func(...string) *testService {
		calls++
		return &testService{}
	}
	err := ValidateConstructors([]any{sliceProvider, variadic})
	if err == nil || !strings.Contains(err.Error(), "constructor[1]") || !strings.Contains(err.Error(), "must be non-variadic") {
		t.Fatalf("의존성 그래프 검사 전에 잘못된 생성자 형식을 보고해야 합니다: %v", err)
	}
	if calls != 0 {
		t.Fatalf("생성자 검증이 provider를 실행하면 안 됩니다: calls=%d", calls)
	}

	fixedSlice := func([]string) *testService {
		calls++
		return &testService{}
	}
	if err := ValidateConstructors([]any{sliceProvider, fixedSlice}); err != nil {
		t.Fatalf("일반 슬라이스 인자를 받는 생성자는 계속 허용해야 합니다: %v", err)
	}
	if calls != 0 {
		t.Fatalf("유효한 생성자 검사도 provider를 실행하면 안 됩니다: calls=%d", calls)
	}
}
