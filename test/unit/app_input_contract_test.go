package spine

import (
	"errors"
	"strings"
	"testing"

	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/query"
)

type anotherBody struct{ Value string }
type contractForm struct {
	Value string `form:"value"`
}
type anotherQuery struct {
	Value int `query:"name"`
}
type anotherHeader struct {
	Value int `header:"x-name"`
}
type pageQuery struct {
	Page string `query:"page"`
}
type partiallyMappedQuery struct {
	Q    string `query:"q"`
	Name string `json:"name"`
}
type typoHeader struct {
	ID string `headers:"X-ID"`
}
type nestedSourceBody struct {
	Inner struct {
		ID string `header:"X-ID"`
	}
}
type paramSourceBody struct {
	ID int `param:"id"`
}
type emptyHeaderSource struct {
	ID string `header:""`
}
type privateQuerySource struct {
	ID string `query:"id"`
}
type hiddenQueryBody struct{ privateQuerySource }
type unsupportedHeaderList struct {
	Values []string `header:"Content-Type"`
}
type duplicateJSONLeft struct{ ID string }
type duplicateJSONRight struct{ ID string }
type duplicateJSONBody struct {
	duplicateJSONLeft
	duplicateJSONRight
}
type hiddenNestedQueryBody struct {
	Inner privateQuerySource `json:"-"`
}
type contractController struct{}

func (*contractController) Bodies(*repeatedBody, *anotherBody)         {}
func (*contractController) BodyForm(*repeatedBody, *contractForm)      {}
func (*contractController) QueryKeys(*repeatedQuery, *anotherQuery)    {}
func (*contractController) HeaderKeys(*repeatedHeader, *anotherHeader) {}
func (*contractController) Pagination(query.Pagination, *pageQuery)    {}
func (*contractController) Missing(*partiallyMappedQuery)              {}
func (*contractController) Typo(*typoHeader)                           {}
func (*contractController) Nested(*nestedSourceBody)                   {}
func (*contractController) Param(*paramSourceBody)                     {}
func (*contractController) EmptyHeader(*emptyHeaderSource)             {}
func (*contractController) HiddenQuery(*hiddenQueryBody)               {}
func (*contractController) UnsupportedHeader(*unsupportedHeaderList)   {}
func (*contractController) Untagged(*anotherBody)                      {}
func (*contractController) DuplicateJSON(*duplicateJSONBody)           {}
func (*contractController) HiddenNestedQuery(*hiddenNestedQueryBody)   {}

func TestHTTPInputContractsFailBeforeBootAndDescriptionCallbacks(t *testing.T) {
	for _, test := range []struct {
		handler any
		code    string
	}{
		{(*contractController).Bodies, "HTTP_BODY_PARAMETERS_DUPLICATED"},
		{(*contractController).BodyForm, "HTTP_BODY_PARAMETERS_DUPLICATED"},
		{(*contractController).QueryKeys, "HTTP_INPUT_KEY_DUPLICATED"},
		{(*contractController).HeaderKeys, "HTTP_INPUT_KEY_DUPLICATED"},
		{(*contractController).Pagination, "HTTP_INPUT_KEY_DUPLICATED"},
		{(*contractController).Missing, "HTTP_INPUT_CONTRACT_INVALID"},
		{(*contractController).Typo, "HTTP_INPUT_CONTRACT_INVALID"},
		{(*contractController).Nested, "HTTP_INPUT_CONTRACT_INVALID"},
		{(*contractController).Param, "HTTP_INPUT_CONTRACT_INVALID"},
		{(*contractController).EmptyHeader, "HTTP_INPUT_CONTRACT_INVALID"},
		{(*contractController).HiddenQuery, "HTTP_INPUT_CONTRACT_INVALID"},
		{(*contractController).UnsupportedHeader, "HTTP_INPUT_CONTRACT_INVALID"},
		{(*contractController).DuplicateJSON, "HTTP_INPUT_CONTRACT_INVALID"},
		{(*contractController).HiddenNestedQuery, "HTTP_INPUT_CONTRACT_INVALID"},
	} {
		for _, configured := range []bool{false, true} {
			for _, disabled := range []bool{false, true} {
				options := []Option{WithHTTPAdapter(&repeatedInputFactory{})}
				if configured {
					options = append(options, WithValidator(&repeatedInputValidator{}))
				}
				app := New(options...)
				app.Constructor(func() *contractController { panic("오류가 있는 앱의 생성자를 호출하면 안 됩니다") })
				app.Route("POST", "/invalid", test.handler)
				bootOptions := boot.Options{HTTP: &boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{Disabled: disabled}}}
				check := func(err error) {
					t.Helper()
					var failure *boot.ConfigError
					if !errors.As(err, &failure) || len(failure.Issues) != 1 || failure.Issues[0].Code != test.code || !strings.HasPrefix(failure.Issues[0].Path, "Routes[0].Parameters[") {
						t.Fatalf("code=%s validator=%v disabled=%v: %v", test.code, configured, disabled, err)
					}
				}
				check(app.Validate(bootOptions))
				prepared, err := app.Prepare(t.Context(), bootOptions)
				if prepared != nil {
					t.Fatal("잘못된 입력 계약으로 앱을 준비하면 안 됩니다")
				}
				check(err)
				check(app.RunContext(t.Context(), bootOptions))
				_, err = DescribeHTTP(app, bootOptions)
				check(err)
			}
		}
	}
}

func TestUntaggedBodyRemainsValidWithoutAValidator(t *testing.T) {
	app := New()
	app.Route("POST", "/plain", (*contractController).Untagged)
	if err := app.Validate(boot.Options{}); err != nil {
		t.Fatal(err)
	}
	description, err := DescribeHTTP(app, boot.Options{HTTP: &boot.HTTPOptions{}})
	if err != nil || !description.PathsOnly || len(description.Routes[0].Inputs) != 0 || len(description.Routes[0].Returns) != 0 || len(description.Schemas) != 0 {
		t.Fatalf("경로만 설명해야 합니다: %+v %v", description, err)
	}
}
