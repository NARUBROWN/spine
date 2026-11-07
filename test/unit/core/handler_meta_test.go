package core

import (
	"reflect"
	"testing"
)

type metadataController struct{}

func (*metadataController) Handle() {}

func TestHandlerNameAcceptsGlobalAndRoutedMetadata(t *testing.T) {
	owner := reflect.TypeFor[*metadataController]()
	method, _ := owner.MethodByName("Handle")
	for _, tc := range []struct {
		name string
		meta HandlerMeta
		want string
	}{
		{"global", HandlerMeta{}, ""},
		{"method", HandlerMeta{ControllerType: owner, Method: method}, owner.String() + ".Handle"},
		{"callback", HandlerMeta{ControllerType: owner, Callback: reflect.ValueOf(func() {})}, owner.String() + ".RegisterRoutes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.meta.HandlerName(); got != tc.want {
				t.Fatalf("name=%q want=%q", got, tc.want)
			}
		})
	}
}
