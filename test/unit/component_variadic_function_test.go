package spine

import (
	"errors"
	"strings"
	"testing"

	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/constructor"
)

func TestCustomFunctionConstructorRejectsVariadicAtValidateAndPrepare(t *testing.T) {
	for _, variadic := range []bool{true, false} {
		t.Run(map[bool]string{true: "variadic", false: "fixed-slice"}[variadic], func(t *testing.T) {
			calls := 0
			var factory any = func([]string) *componentService {
				calls++
				return &componentService{}
			}
			if variadic {
				factory = func(...string) *componentService {
					calls++
					return &componentService{}
				}
			}
			app := New()
			app.Constructor(
				constructor.Custom(func() []string {
					calls++
					return []string{"one", "two"}
				}),
				constructor.Custom(factory),
			)
			check := func(err error) {
				t.Helper()
				if !variadic {
					if err != nil {
						t.Fatalf("일반 슬라이스 인자 생성자는 허용해야 합니다: %v", err)
					}
					return
				}
				var failure *boot.ConfigError
				if !errors.As(err, &failure) || len(failure.Issues) != 1 ||
					failure.Issues[0].Code != "COMPONENT_CONSTRUCTOR_INVALID" ||
					!strings.Contains(failure.Issues[0].Message, "must be non-variadic") {
					t.Fatalf("가변 인자 함수를 생성 계약 오류로 거부해야 합니다: %v", err)
				}
			}
			check(app.Validate(boot.Options{}))
			prepared, err := app.Prepare(t.Context(), boot.Options{})
			check(err)
			if variadic && prepared != nil {
				t.Fatal("잘못된 생성자로 앱이 준비되면 안 됩니다")
			}
			if !variadic {
				if prepared == nil {
					t.Fatal("유효한 생성자로 앱을 준비해야 합니다")
				}
				if err := prepared.Close(t.Context()); err != nil {
					t.Fatal(err)
				}
			}
			if calls != 0 {
				t.Fatalf("생성자 검증이 provider를 실행하면 안 됩니다: calls=%d", calls)
			}
		})
	}
}
