package spine

import (
	"errors"
	"strings"
	"testing"

	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/path"
)

type pathContract struct {
	ID int64 `path:"id"`
}
type otherPathContract struct {
	ID string `path:"id"`
}
type badPathContract struct {
	ID []int `path:"id"`
}
type mixedPathContract struct {
	ID int    `path:"id"`
	Q  string `query:"q"`
}
type nestedPathBody struct {
	Hidden pathContract `json:"-"`
}
type pathContractController struct{}

func (*pathContractController) One(*pathContract)                           {}
func (*pathContractController) Repeated(*pathContract, *pathContract)       {}
func (*pathContractController) Collision(*pathContract, *otherPathContract) {}
func (*pathContractController) Bad(*badPathContract)                        {}
func (*pathContractController) Mixed(*mixedPathContract)                    {}
func (*pathContractController) Value(pathContract)                          {}
func (*pathContractController) Body(*nestedPathBody)                        {}
func (*pathContractController) Legacy(path.Int, *pathContract)              {}

func (*pathContractController) TooManyLegacy(path.Int, path.Int) {}

func TestPathConfigurationFailsBeforeInitializationAndSchemaCallbacks(t *testing.T) {
	for _, test := range []struct {
		handler     any
		route, code string
	}{
		{(*pathContractController).One, "/:other", "HTTP_PATH_KEY_UNDECLARED"},
		{(*pathContractController).One, "/:ID", "HTTP_PATH_KEY_UNDECLARED"},
		{(*pathContractController).One, "/:id/items/:id", "HTTP_PATH_TEMPLATE_INVALID"},
		{(*pathContractController).One, "/:/items", "HTTP_PATH_TEMPLATE_INVALID"},
		{(*pathContractController).Repeated, "/:id", "HTTP_INPUT_DTO_DUPLICATED"},
		{(*pathContractController).Collision, "/:id", "HTTP_INPUT_KEY_DUPLICATED"},
		{(*pathContractController).Bad, "/:id", "HTTP_INPUT_CONTRACT_INVALID"},
		{(*pathContractController).Mixed, "/:id", "HTTP_INPUT_CONTRACT_INVALID"},
		{(*pathContractController).Value, "/:id", "HTTP_INPUT_CONTRACT_INVALID"},
		{(*pathContractController).Body, "/:id", "HTTP_INPUT_CONTRACT_INVALID"},
		{(*pathContractController).Legacy, "/:id", "HTTP_PATH_INPUT_MIXED"},
		{(*pathContractController).TooManyLegacy, "/:id", "HTTP_PATH_ARGUMENT_UNBOUND"},
	} {
		for _, configured := range []bool{false, true} {
			for _, enabled := range []bool{false, true} {
				opts := []Option{WithHTTPAdapter(&repeatedInputFactory{})}
				if configured {
					opts = append(opts, WithValidator(&repeatedInputValidator{}))
				}
				app := New(opts...)
				app.Constructor(func() *pathContractController {
					panic("부팅 검사보다 먼저 생성자를 실행하면 안 됩니다")
				})
				app.Route("GET", test.route, test.handler)
				options := boot.Options{HTTP: &boot.HTTPOptions{GlobalPrefix: "/api", OpenAPI: boot.OpenAPIOptions{Enabled: enabled}}}
				check := func(err error) {
					t.Helper()
					var failure *boot.ConfigError
					if !errors.As(err, &failure) || len(failure.Issues) != 1 || failure.Issues[0].Code != test.code || !strings.HasPrefix(failure.Issues[0].Path, "Routes[0].") {
						t.Fatalf("경로 설정 오류 %s: %v", test.code, err)
					}
				}
				check(app.Validate(options))
				p, err := app.Prepare(t.Context(), options)
				if p != nil {
					t.Fatal("잘못된 경로 계약으로 준비하면 안 됩니다")
				}
				check(err)
				check(app.RunContext(t.Context(), options))
				_, err = DescribeHTTP(app, options)
				check(err)
			}
		}
	}
}

func TestPathDTOAllowsUnusedRouteKeysAndReuseAcrossRoutes(t *testing.T) {
	app := New()
	app.Route("GET", "/orgs/:org/items/:id", (*pathContractController).One)
	app.Route("GET", "/items/:id", (*pathContractController).One)
	if err := app.Validate(boot.Options{}); err != nil {
		t.Fatal(err)
	}
	desc, err := DescribeHTTP(app, boot.Options{HTTP: &boot.HTTPOptions{}})
	if err != nil || !desc.PathsOnly || len(desc.Routes) != 2 {
		t.Fatalf("미등록 검증기의 경로 전용 명세: %+v %v", desc, err)
	}
}
