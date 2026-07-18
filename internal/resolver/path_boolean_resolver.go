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

func (r *PathBooleanResolver) Resolve(ctx core.ExecutionContext, parameterMeta ParameterMeta) (any, error) {
	httpCtx, ok := ctx.(core.HttpRequestContext)
	if !ok {
		return nil, fmt.Errorf("context is not an HTTP request context")
	}

	if parameterMeta.PathKey == "" {
		return nil, fmt.Errorf(
			"path key is not bound: %v",
			parameterMeta.Type,
		)
	}

	raw, ok := httpCtx.Params()[parameterMeta.PathKey]
	if !ok {
		return nil, fmt.Errorf(
			"path parameter not found: %s",
			parameterMeta.PathKey,
		)
	}

	value, err := parseBool(raw)
	if err != nil {
		return nil, fmt.Errorf(
			"invalid path parameter: %s (%v)",
			parameterMeta.PathKey,
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
