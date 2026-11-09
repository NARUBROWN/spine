package resolver

import (
	"context"
	"reflect"
	"testing"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/input"
)

type pathQuery struct{ Limit int }
type pathBody struct{ Name string }
type multiPathController struct{}

func (*multiPathController) Handle(context.Context, input.Path[int64], input.Query[pathQuery], input.Path[string], input.Body[pathBody]) {
}

func TestScalarPathArgumentsFollowRouteOrder(t *testing.T) {
	method, _ := reflect.TypeFor[*multiPathController]().MethodByName("Handle")
	handlers := []core.HandlerMeta{
		{Method: method},
		{Callback: reflect.ValueOf(func(context.Context, input.Path[int64], input.Query[pathQuery], input.Path[string], input.Body[pathBody]) {
		})},
	}
	for _, meta := range handlers {
		for _, keys := range [][]string{{"id", "name"}, {"first", "second"}, {"id"}, nil} {
			parameters := HandlerParameters(meta, keys)
			for i, parameter := range parameters {
				want := ""
				if i == 1 && len(keys) > 0 {
					want = keys[0]
				}
				if i == 3 && len(keys) > 1 {
					want = keys[1]
				}
				if parameter.PathKey != want || parameter.Index != i {
					t.Fatalf("keys=%v parameter=%d: got %+v, want key %q", keys, i, parameter, want)
				}
			}
		}
	}
}
