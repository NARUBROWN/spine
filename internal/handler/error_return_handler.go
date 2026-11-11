package handler

import (
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/internal/execution"
)

type ErrorReturnHandler struct{}

func (h *ErrorReturnHandler) Supports(returnType reflect.Type) bool {
	errorType := reflect.TypeFor[error]()
	return returnType.Implements(errorType)
}

func (h *ErrorReturnHandler) Handle(value any, ctx execution.Context) error {
	rw, writerErr := responseWriter(ctx)
	if writerErr != nil {
		return writerErr
	}

	err, ok := value.(error)
	if !ok {
		return fmt.Errorf("ErrorReturnHandler only supports error values: %T", value)
	}

	return WriteErrorResponse(rw, err)
}
