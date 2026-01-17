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
	rwAny, ok := ctx.Get("spine.response_writer")
	if !ok {
		return fmt.Errorf("ExecutionContext 안에서 ResponseWriter를 찾을 수 없습니다.")
	}

	rw, ok := rwAny.(core.ResponseWriter)
	if !ok {
		return fmt.Errorf("ResponseWriter 타입이 올바르지 않습니다.")
	}

	err, ok := value.(error)
	if !ok {
		return fmt.Errorf("ErrorReturnHandler는 error 타입만 처리할 수 있습니다: %T", value)
	}

	status := 500
	message := err.Error()

	// HTTPError면 상태 코드를 추출한다.
	var httpErr *httperr.HTTPError
	if errors.As(err, &httpErr) {
		status = httpErr.Status
		message = httpErr.Message
	}

	return rw.WriteJSON(status, map[string]any{
		"message": message,
	})
}
