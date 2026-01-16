package resolver

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/path"
)

type PathBooleanResolver struct{}

func (r *PathBooleanResolver) Supports(parameterMeta ParameterMeta) bool {
	return parameterMeta.Type == reflect.TypeFor[path.Boolean]()
}

func (r *PathBooleanResolver) Resolve(ctx core.Context, parameterMeta ParameterMeta) (any, error) {
	raw, ok := ctx.Params()[parameterMeta.Type.Name()]
	if !ok {
		return nil, fmt.Errorf("Path param을 찾을 수 없습니다. %s", parameterMeta.Type.Name())
	}

	value, err := parseBool(raw)
	if err != nil {
		return nil, fmt.Errorf(
			"유효하지 않은 Path param입니다. %s: %v",
			parameterMeta.Type.Name(),
			err,
		)
	}

	return path.Boolean{Value: value}, nil
}

func parseBool(s string) (bool, error) {
	switch strings.ToLower(s) {
	case "true", "1", "yes", "y", "on":
		return true, nil
	case "false", "0", "no", "n", "off":
		return false, nil
	default:
		return false, fmt.Errorf("not a boolean: %s", s)
	}
}
