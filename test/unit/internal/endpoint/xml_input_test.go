package endpoint

import (
	"errors"
	"reflect"
	"testing"

	"github.com/NARUBROWN/spine/internal/configuration"
	"github.com/NARUBROWN/spine/internal/router"
	"github.com/NARUBROWN/spine/pkg/input"
)

func TestDescriptionRejectsConflictingXMLBody(t *testing.T) {
	handler := func(input.Body[struct {
		First  string `json:"first" xml:"same"`
		Second string `json:"second" xml:"same>child"`
	}]) {
		panic("명세 조회에서 핸들러를 실행하면 안 됩니다")
	}
	_, err := Describe([]router.RouteSpec{{
		Method: "POST", Path: "/xml", Handler: handler, ControllerType: reflect.TypeFor[*controller](),
	}}, &configuration.HTTPOptions{}, nil)
	var failure *configuration.ConfigError
	if !errors.As(err, &failure) || len(failure.Issues) != 1 || failure.Issues[0].Code != "HTTP_INPUT_CONTRACT_INVALID" {
		t.Fatalf("실행할 수 없는 XML 계약을 명세에서 거부해야 합니다: %v", err)
	}
}
