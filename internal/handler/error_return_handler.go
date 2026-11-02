package handler

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/httperr"
)

type ErrorReturnHandler struct{}

func (h *ErrorReturnHandler) Supports(returnType reflect.Type) bool {
	errorType := reflect.TypeFor[error]()
	return returnType.Implements(errorType)
}

func (h *ErrorReturnHandler) Handle(value any, ctx core.ExecutionContext) error {
	rw, writerErr := responseWriter(ctx)
	if writerErr != nil {
		return writerErr
	}

	err, ok := value.(error)
	if !ok {
		return fmt.Errorf("ErrorReturnHandler only supports error values: %T", value)
	}

	status := 500
	message := "Internal server error"

	// HTTPError면 상태 코드를 추출한다.
	var httpErr *httperr.HTTPError
	if errors.As(err, &httpErr) {
		status = httpErr.Status
		message = httpErr.Message
	}

	return rw.WriteJSON(status, ErrorJSON(message))
}
