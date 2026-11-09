package endpoint

import (
	"reflect"
	"testing"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/input"
	"github.com/NARUBROWN/spine/pkg/path"
)

type scalarDTO struct {
	ID int `path:"id"`
}

func TestScalarPathPreparationContracts(t *testing.T) {
	for _, tc := range []struct {
		name, route, code string
		handler           any
	}{
		{"one", "/users/:id", "", func(input.Path[int64]) {}},
		{"none", "/users", "HTTP_PATH_SCALAR_AMBIGUOUS", func(input.Path[int64]) {}},
		{"multiple", "/:org/:id", "HTTP_PATH_SCALAR_AMBIGUOUS", func(input.Path[int64]) {}},
		{"duplicate", "/:id", "HTTP_PATH_SCALAR_AMBIGUOUS", func(input.Path[int], input.Path[int64]) {}},
		{"dto conflict", "/:id", "HTTP_INPUT_KEY_DUPLICATED", func(input.Path[int], input.Path[scalarDTO]) {}},
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
