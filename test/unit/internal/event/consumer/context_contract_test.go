package consumer

import (
	"context"
	"reflect"
	"testing"

	"github.com/NARUBROWN/spine/pkg/spine"
)

// Check the concrete object: narrowing its static interface alone cannot hide controls.
func assertMessageContextContract(t *testing.T, ctx any) {
	t.Helper()
	_, closeVoid := ctx.(interface{ Close() })
	_, closeError := ctx.(interface{ Close() error })
	_, closeContext := ctx.(interface{ Close(context.Context) error })
	_, cancel := ctx.(interface{ Cancel() })
	_, stop := ctx.(interface{ Stop() error })
	_, release := ctx.(interface{ ReleaseStore() })
	_, ack := ctx.(interface{ Ack() error })
	_, nack := ctx.(interface{ Nack() error })
	for name, exposed := range map[string]bool{
		"Close()": closeVoid, "Close() error": closeError,
		"Close(context.Context) error": closeContext, "Cancel": cancel,
		"Stop": stop, "ReleaseStore": release, "Ack": ack, "Nack": nack,
	} {
		if exposed {
			t.Errorf("context %T exposes runtime control %s through type assertion", ctx, name)
		}
	}
	actual := reflect.TypeOf(ctx)
	contract := reflect.TypeFor[spine.EventCtx]()
	for n := 0; n < actual.NumMethod(); n++ {
		name := actual.Method(n).Name
		if _, allowed := contract.MethodByName(name); !allowed {
			t.Errorf("context %T exposes method outside its public contract: %s", ctx, name)
		}
	}
}
