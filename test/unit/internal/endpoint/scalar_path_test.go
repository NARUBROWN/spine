package endpoint

import (
	"context"
	"reflect"
	"testing"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/input"
	"github.com/NARUBROWN/spine/pkg/path"
)

type scalarDTO struct {
	ID int `path:"id"`
}

type scalarQuery struct{ Limit int }
type scalarBody struct{ Name string }

func TestScalarPathPreparationContracts(t *testing.T) {
	for _, tc := range []struct {
		name, route, code string
		handler           any
	}{
		{"one", "/users/:id", "", func(input.Path[int64]) {}},
		{"multiple", "/:id/:name", "", func(input.Path[int64], input.Path[string]) {}},
		{"same type", "/:first/:second", "", func(input.Path[string], input.Path[string]) {}},
		{"interleaved", "/:id/:name", "", func(context.Context, input.Path[int64], input.Query[scalarQuery], input.Path[string], input.Body[scalarBody]) {
		}},
		{"dto with separate key", "/:id/:name", "HTTP_PATH_INPUT_MIXED", func(input.Path[string], input.Path[scalarDTO]) {}},
		{"too many", "/:id/:name", "HTTP_PATH_SCALAR_AMBIGUOUS", func(input.Path[int], input.Path[string], input.Path[bool]) {}},
		{"none", "/users", "HTTP_PATH_SCALAR_AMBIGUOUS", func(input.Path[int64]) {}},
		{"too few", "/:org/:id", "HTTP_PATH_SCALAR_AMBIGUOUS", func(input.Path[int64]) {}},
		{"duplicate", "/:id", "HTTP_PATH_SCALAR_AMBIGUOUS", func(input.Path[int], input.Path[int64]) {}},
		{"dto conflict", "/:id", "HTTP_PATH_INPUT_MIXED", func(input.Path[int], input.Path[scalarDTO]) {}},
		{"legacy conflict", "/:id", "HTTP_PATH_INPUT_MIXED", func(input.Path[int], path.Int) {}},
		{"unsupported", "/:id", "HTTP_INPUT_CONTRACT_INVALID", func(input.Path[[]int]) {}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			meta := core.HandlerMeta{Callback: reflect.ValueOf(tc.handler)}
			issues := HandlerDTOParameterIssues(meta, "Routes[0]", tc.route)
			if tc.code == "" && len(issues) != 0 {
				t.Fatalf("%+v", issues)
			}
			if tc.code != "" {
				for _, issue := range issues {
					if issue.Code == tc.code {
						return
					}
				}
				t.Fatalf("expected %s: %+v", tc.code, issues)
			}
		})
	}
}
