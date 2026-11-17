package execution

import (
	"errors"
	"fmt"
	"runtime/debug"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/contextview"
)

// BeforeResponse는 사전 처리를 마친 인터셉터를 역순으로 실행합니다.
func BeforeResponse(ctx contextview.Ctx, meta HandlerMeta, interceptors []core.Interceptor, finalErr error) (error, error) {
	var lastErr error
	for i := len(interceptors) - 1; i >= 0; i-- {
		if err := callBeforeResponse(interceptors[i], ctx, meta, finalErr); err != nil {
			finalErr = errors.Join(finalErr, err)
			lastErr = err
		}
	}
	return finalErr, lastErr
}

// AfterCompletion은 완료 훅을 역순으로 실행하고 패닉을 최종 오류에 추가합니다.
func AfterCompletion(ctx contextview.Ctx, meta HandlerMeta, interceptors []core.Interceptor, finalErr error) error {
	for i := len(interceptors) - 1; i >= 0; i-- {
		if err := callAfterCompletion(interceptors[i], ctx, meta, finalErr); err != nil {
			finalErr = errors.Join(finalErr, err)
		}
	}
	return finalErr
}

func callBeforeResponse(
	interceptor core.Interceptor,
	ctx contextview.Ctx,
	meta HandlerMeta,
	executionErr error,
) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = PanicError(recovered)
		}
	}()
	return interceptor.BeforeResponse(ctx, meta.View(), executionErr)
}

func callAfterCompletion(
	interceptor core.Interceptor,
	ctx contextview.Ctx,
	meta HandlerMeta,
	executionErr error,
) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = PanicError(recovered)
		}
	}()
	interceptor.AfterCompletion(ctx, meta.View(), executionErr)
	return nil
}

func PanicError(recovered any) error {
	if err, ok := recovered.(error); ok {
		return fmt.Errorf("panic recovered: %w\n%s", err, debug.Stack())
	}
	return fmt.Errorf("panic recovered: %v\n%s", recovered, debug.Stack())
}
