package resolver

import (
	"fmt"
	"github.com/NARUBROWN/spine/internal/inputerr"
	"net/http"
	"reflect"
	"strconv"

	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/httpfailure"
	"github.com/NARUBROWN/spine/pkg/path"
)

type PathIntResolver struct{}

func (r *PathIntResolver) Supports(parameterMeta ParameterMeta) bool {
	return parameterMeta.Type == reflect.TypeFor[path.Int]()
}

func (r *PathIntResolver) Resolve(ctx execution.Context, parameterMeta ParameterMeta) (any, error) {
	httpCtx, ok := ctx.(execution.HTTPRequestContext)
	if !ok {
		return nil, fmt.Errorf("context is not an HTTP request context")
	}

	if parameterMeta.PathKey == "" {
		return nil, fmt.Errorf("no path key matches %v", parameterMeta.Type)
	}
	raw, ok := httpCtx.Params()[parameterMeta.PathKey]
	if !ok {
		return nil, fmt.Errorf("path parameter not found: %s", parameterMeta.PathKey)
	}

	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, &inputerr.Error{Source: "path", Cause: &httpfailure.HTTPError{
			Status:  http.StatusBadRequest,
			Message: fmt.Sprintf("Invalid path parameter: %s", parameterMeta.PathKey),
			Cause:   err,
		}}
	}

	return path.Int{Value: value}, nil
}
