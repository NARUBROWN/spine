package spine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/header"
	"github.com/NARUBROWN/spine/pkg/path"
	"github.com/NARUBROWN/spine/pkg/query"
)

type legacyInputController struct{}

func (*legacyInputController) QueryQuery(query.Values, *repeatedQuery)     {}
func (*legacyInputController) QueryHeader(query.Values, *repeatedHeader)   {}
func (*legacyInputController) QueryPath(query.Values, *pathContract)       {}
func (*legacyInputController) HeaderQuery(header.Values, *repeatedQuery)   {}
func (*legacyInputController) HeaderHeader(header.Values, *repeatedHeader) {}
func (*legacyInputController) HeaderPath(header.Values, *pathContract)     {}
func (*legacyInputController) PathQuery(path.Int, *repeatedQuery)          {}
func (*legacyInputController) PathHeader(path.Int, *repeatedHeader)        {}
func (*legacyInputController) PathPath(path.Int, *pathContract)            {}
func (*legacyInputController) NewFirst(*repeatedQuery, header.Values)      {}
func (*legacyInputController) QueryPointer(*query.Values)                  {}
func (*legacyInputController) HeaderPointer(*header.Values)                {}
func (*legacyInputController) Legacy(context.Context, core.ControllerContext, path.Int, query.Values, header.Values, query.Pagination, *repeatedBody) {
}
func (*legacyInputController) Typed(*pathContract, *repeatedQuery, *repeatedHeader) {
}
func (*legacyInputController) PaginationPath(query.Pagination, *pathContract)        {}
func (*legacyInputController) PaginationQuery(query.Pagination, *repeatedQuery)      {}
func (*legacyInputController) PaginationHeader(query.Pagination, *repeatedHeader)    {}
func (*legacyInputController) NewBeforePagination(*repeatedHeader, query.Pagination) {}
func (*legacyInputController) PaginationPointer(*query.Pagination)                   {}
func (*legacyInputController) RepeatedPagination(query.Pagination, query.Pagination) {
}

func TestDeprecatedInputsCannotMixWithAnyNewSourceDTO(t *testing.T) {
	for _, test := range []struct {
		handler any
		code    string
	}{
		{(*legacyInputController).QueryQuery, "HTTP_INPUT_API_MIXED"},
		{(*legacyInputController).QueryHeader, "HTTP_INPUT_API_MIXED"},
		{(*legacyInputController).QueryPath, "HTTP_INPUT_API_MIXED"},
		{(*legacyInputController).HeaderQuery, "HTTP_INPUT_API_MIXED"},
		{(*legacyInputController).HeaderHeader, "HTTP_INPUT_API_MIXED"},
		{(*legacyInputController).HeaderPath, "HTTP_INPUT_API_MIXED"},
		{(*legacyInputController).PathQuery, "HTTP_INPUT_API_MIXED"},
		{(*legacyInputController).PathHeader, "HTTP_INPUT_API_MIXED"},
		{(*legacyInputController).PathPath, "HTTP_PATH_INPUT_MIXED"},
		{(*legacyInputController).NewFirst, "HTTP_INPUT_API_MIXED"},
		{(*legacyInputController).QueryPointer, "HTTP_INPUT_CONTRACT_INVALID"},
		{(*legacyInputController).HeaderPointer, "HTTP_INPUT_CONTRACT_INVALID"},
		{(*legacyInputController).PaginationPath, "HTTP_INPUT_API_MIXED"},
		{(*legacyInputController).PaginationQuery, "HTTP_INPUT_API_MIXED"},
		{(*legacyInputController).PaginationHeader, "HTTP_INPUT_API_MIXED"},
		{(*legacyInputController).NewBeforePagination, "HTTP_INPUT_API_MIXED"},
		{(*legacyInputController).PaginationPointer, "HTTP_INPUT_CONTRACT_INVALID"},
	} {
		for _, configured := range []bool{false, true} {
			for _, disabled := range []bool{false, true} {
				opts := []Option{WithHTTPAdapter(&repeatedInputFactory{})}
				if configured {
					opts = append(opts, WithValidator(&repeatedInputValidator{}))
				}
				app := New(opts...)
				app.Constructor(func() *legacyInputController { panic("부팅 검사 전에 초기화하면 안 됩니다") })
				app.Route("GET", "/:id", test.handler)
				options := boot.Options{HTTP: &boot.HTTPOptions{GlobalPrefix: "/api", OpenAPI: boot.OpenAPIOptions{Disabled: disabled}}}
				check := func(err error) {
					t.Helper()
					var failure *boot.ConfigError
					if !errors.As(err, &failure) || len(failure.Issues) != 1 || failure.Issues[0].Code != test.code {
						t.Fatalf("code=%s validator=%v disabled=%v: %v", test.code, configured, disabled, err)
					}
				}
				check(app.Validate(options))
				prepared, err := app.Prepare(t.Context(), options)
				if prepared != nil {
					t.Fatal("혼용한 앱을 준비하면 안 됩니다")
				}
				check(err)
				check(app.RunContext(t.Context(), options))
				_, err = DescribeHTTP(app, options)
				check(err)
			}
		}
	}
}

func TestDeprecatedInputsCanKeepExistingBodyContextsAndPagination(t *testing.T) {
	app := New()
	app.Route("POST", "/:id", (*legacyInputController).Legacy)
	app.Route("GET", "/new/:id", (*legacyInputController).Typed)
	if err := app.Validate(boot.Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := DescribeHTTP(app, boot.Options{HTTP: &boot.HTTPOptions{GlobalPrefix: "/api"}}); err != nil {
		t.Fatal(err)
	}
}

func TestDeprecatedPaginationKeepsDuplicateKeyChecks(t *testing.T) {
	app := New(WithHTTPAdapter(&repeatedInputFactory{}))
	app.Route("GET", "/legacy", (*legacyInputController).RepeatedPagination)
	err := app.Validate(boot.Options{HTTP: &boot.HTTPOptions{}})
	var failure *boot.ConfigError
	if !errors.As(err, &failure) || len(failure.Issues) != 2 {
		t.Fatalf("기존 page와 size의 중복을 각각 거부해야 합니다: %v", err)
	}
	for i, name := range []string{"page", "size"} {
		issue := failure.Issues[i]
		if issue.Code != "HTTP_INPUT_KEY_DUPLICATED" || issue.Path != "Routes[0].Parameters[1]" || !strings.Contains(issue.Message, name) {
			t.Fatalf("기존 페이지네이션의 키 중복 검사: %+v", issue)
		}
	}
}
