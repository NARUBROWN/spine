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

func (r *PathBooleanResolver) Resolve(ctx core.RequestContext, parameterMeta ParameterMeta) (any, error) {
	httpCtx, ok := ctx.(core.HttpRequestContext)
	if !ok {
		return nil, fmt.Errorf("HTTP 요청 컨텍스트가 아닙니다")
	}

	if parameterMeta.PathKey == "" {
		return nil, fmt.Errorf(
			"path key가 바인딩되지 않았습니다: %v",
			parameterMeta.Type,
		)
	}

	raw, ok := httpCtx.Params()[parameterMeta.PathKey]
	if !ok {
		return nil, fmt.Errorf(
			"path param을 찾을 수 없습니다: %s",
			parameterMeta.PathKey,
		)
	}

	value, err := parseBool(raw)
	if err != nil {
		return nil, fmt.Errorf(
			"유효하지 않은 path param입니다: %s (%v)",
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
