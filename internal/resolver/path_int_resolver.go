package resolver

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/path"
)

type PathIntResolver struct{}

func (r *PathIntResolver) Supports(parameterMeta ParameterMeta) bool {
	return parameterMeta.Type == reflect.TypeFor[path.Int]()
}

func (r *PathIntResolver) Resolve(ctx core.Context, parameterMeta ParameterMeta) (any, error) {
	raw, ok := ctx.Params()[parameterMeta.Type.Name()]
	if !ok {
		return nil, fmt.Errorf("path param을 찾을 수 없습니다. %s", parameterMeta.Type.Name())
	}

	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, fmt.Errorf(
			"유효하지 않은 Path param %s: %v",
			parameterMeta.Type.Name(),
			err,
		)
	}

	return path.Int{Value: value}, nil
}
