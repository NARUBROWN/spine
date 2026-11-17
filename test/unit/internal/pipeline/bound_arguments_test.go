package pipeline

import (
	"context"
	"errors"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/container"
	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/invoker"
	"github.com/NARUBROWN/spine/internal/resolver"
	"github.com/NARUBROWN/spine/pkg/spine"
	"reflect"
	"slices"
	"testing"
)

type bindingObserver struct {
	events *[]string
	reject bool
}

func (o *bindingObserver) PreHandle(ctx spine.Ctx, _ core.HandlerMeta) error {
	*o.events = append(*o.events, "pre")
	ctx.ExecutionContext().Set("authorized", true)
	if o.reject {
		return errors.New("unauthorized")
	}
	return nil
}
func (o *bindingObserver) PostHandle(spine.Ctx, core.HandlerMeta) {
	*o.events = append(*o.events, "post")
}
func (o *bindingObserver) BeforeResponse(spine.Ctx, core.HandlerMeta, error) error {
	*o.events = append(*o.events, "before")
	return nil
}
func (o *bindingObserver) AfterCompletion(ctx spine.Ctx, _ core.HandlerMeta, _ error) {
	if value, _ := ctx.ExecutionContext().Get("authorized"); value != true {
		panic("store released before completion")
	}
	*o.events = append(*o.events, "complete")
}

func TestTransportBindingRunsAfterAuthorizationAndPreservesCompletion(t *testing.T) {
	for _, outcome := range []string{"success", "reject", "binding-error", "binding-panic"} {
		t.Run(outcome, func(t *testing.T) {
			events := []string{}
			state := execution.NewState(context.Background())
			run := NewPipeline(nil, invoker.NewInvoker(container.New()))
			run.AddInterceptor(&bindingObserver{events: &events, reject: outcome == "reject"})
			meta := execution.HandlerMeta{Callback: reflect.ValueOf(func(value int) error {
				if value != 42 {
					t.Fatal(value)
				}
				events = append(events, "handler")
				return nil
			})}
			err := run.ExecuteBound(state, meta, func(parameters []resolver.ParameterMeta) ([]any, error) {
				events = append(events, "bind")
				if value, _ := state.Get("authorized"); value != true {
					t.Fatal("binding precedes authorization")
				}
				if len(parameters) != 1 || parameters[0].Type != reflect.TypeFor[int]() {
					t.Fatal(parameters)
				}
				switch outcome {
				case "binding-error":
					return nil, errors.New("decode failed")
				case "binding-panic":
					panic("decode panic")
				}
				return []any{42}, nil
			})
			if (err != nil) != (outcome != "success") {
				t.Fatal(err)
			}
			want := []string{"pre", "bind", "handler", "post", "before", "complete"}
			if outcome == "reject" {
				want = []string{"pre", "complete"}
			} else if outcome != "success" {
				want = []string{"pre", "bind", "before", "complete"}
			}
			if !slices.Equal(events, want) {
				t.Fatalf("events=%v want=%v", events, want)
			}
			if _, ok := state.Get("authorized"); ok {
				t.Fatal("completed store retained")
			}
		})
	}
}
