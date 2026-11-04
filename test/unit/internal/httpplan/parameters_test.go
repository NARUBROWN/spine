package httpplan

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/NARUBROWN/spine/core"
	left "github.com/NARUBROWN/spine/internal/httpplan/testdata/left"
	right "github.com/NARUBROWN/spine/internal/httpplan/testdata/right"
	"github.com/NARUBROWN/spine/pkg/path"
)

type bodyDTO struct{ Name string }
type bodyAlias = bodyDTO
type otherBody bodyDTO
type queryDTO struct {
	Name string `query:"name"`
}
type headerDTO struct {
	Name string `header:"X-Name"`
}
type formDTO struct {
	Name string `form:"name"`
}
type controller struct{}

func (*controller) Body(*bodyDTO, *bodyDTO)                        {}
func (*controller) Query(*queryDTO, *queryDTO)                     {}
func (*controller) Header(*headerDTO, *headerDTO)                  {}
func (*controller) Form(*formDTO, *formDTO)                        {}
func (*controller) Alias(*bodyDTO, *bodyAlias)                     {}
func (*controller) SeparateTypes(*bodyDTO, *otherBody)             {}
func (*controller) SeparatePackages(*left.Request, *right.Request) {}
func (*controller) Contexts(context.Context, context.Context, core.ControllerContext, core.ControllerContext) {
}
func (*controller) Paths(path.Int, path.Int) {}
func (*controller) AfterContext(context.Context, *bodyDTO, *bodyDTO) {
}
func (*controller) Anonymous(*struct{ Name string }, *struct{ Name string }) {}

func TestDTOParameterIssuesUseTypeIdentityAndExcludeNonDTOBindings(t *testing.T) {
	for _, test := range []struct {
		method string
		index  int
	}{
		{"Body", 1}, {"Query", 1}, {"Header", 1}, {"Form", 1}, {"Alias", 1},
		{"AfterContext", 2}, {"Anonymous", 1},
		{"SeparateTypes", 1}, {"SeparatePackages", 1}, {"Contexts", -1}, {"Paths", -1},
	} {
		t.Run(test.method, func(t *testing.T) {
			method, ok := reflect.TypeFor[*controller]().MethodByName(test.method)
			if !ok {
				t.Fatal("메서드를 찾을 수 없습니다")
			}
			issues := DTOParameterIssues(method, "Routes[3]", "/:id/:name")
			if test.index < 0 {
				if len(issues) != 0 {
					t.Fatalf("서로 다른 타입이나 비 DTO 입력은 중복이 아닙니다: %+v", issues)
				}
				return
			}
			code := "HTTP_INPUT_DTO_DUPLICATED"
			if test.method == "SeparateTypes" || test.method == "SeparatePackages" {
				code = "HTTP_BODY_PARAMETERS_DUPLICATED"
			}
			if len(issues) != 1 || issues[0].Code != code || !strings.Contains(issues[0].Message, test.method) || issues[0].Hint == "" {
				t.Fatalf("중복 타입과 핸들러를 안내해야 합니다: %+v", issues)
			}
			want := "Routes[3].Parameters[1]"
			if test.index == 2 {
				want = "Routes[3].Parameters[2]"
			}
			if issues[0].Path != want {
				t.Fatalf("receiver를 제외한 실제 인자 위치여야 합니다: %s", issues[0].Path)
			}
		})
	}
}
