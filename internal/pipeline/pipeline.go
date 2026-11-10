package pipeline

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"runtime/debug"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/contextview"
	"github.com/NARUBROWN/spine/internal/event/hook"
	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/handler"
	"github.com/NARUBROWN/spine/internal/httpfailure"
	"github.com/NARUBROWN/spine/internal/invoker"
	"github.com/NARUBROWN/spine/internal/resolver"
	"github.com/NARUBROWN/spine/internal/router"
)

type Pipeline struct {
	router            router.Router
	interceptors      []core.Interceptor
	argumentResolvers []resolver.ArgumentResolver
	returnHandlers    []handler.ReturnValueHandler
	invoker           *invoker.Invoker
	postHooks         []hook.PostExecutionHook
}

func NewPipeline(router router.Router, invoker *invoker.Invoker) *Pipeline {
	return &Pipeline{
		router:  router,
		invoker: invoker,
	}
}

func (p *Pipeline) AddInterceptor(its ...core.Interceptor) {
	p.interceptors = append(p.interceptors, its...)
}

func (p *Pipeline) AddArgumentResolver(resolvers ...resolver.ArgumentResolver) {
	p.argumentResolvers = append(p.argumentResolvers, resolvers...)
}

func (p *Pipeline) AddReturnValueHandler(handlers ...handler.ReturnValueHandler) {
	p.returnHandlers = append(p.returnHandlers, handlers...)
}

// Execute는 하나의 요청 실행 전체를 소유합니다.
func (p *Pipeline) Execute(ctx execution.Context) (finalErr error) {
	// 응답 처리와 완료 훅이 끝난 뒤 저장값의 참조를 해제합니다.
	if store, ok := ctx.(interface{ ReleaseStore() }); ok {
		defer store.ReleaseStore()
	}
	globalMeta := execution.HandlerMeta{}
	var interceptorCtx contextview.Ctx
	if len(p.interceptors) > 0 {
		interceptorCtx = execution.NewContext(ctx)
	}
	var routeInterceptors []core.Interceptor
	var globalFinalizers []core.Interceptor
	var routeFinalizers []core.Interceptor
	var results []any
	var returnedErr error
	beforeResponseCalled := false
	responseStage := stageResponseWriter(ctx)

	// AfterCompletion은 응답 처리가 끝난 뒤 실행한다.
	defer func() {
		for i := len(routeInterceptors) - 1; i >= 0; i-- {
			if err := callAfterCompletion(routeInterceptors[i], interceptorCtx, globalMeta, finalErr); err != nil {
				finalErr = errors.Join(finalErr, err)
			}
		}
		for i := len(p.interceptors) - 1; i >= 0; i-- {
			if err := callAfterCompletion(p.interceptors[i], interceptorCtx, globalMeta, finalErr); err != nil {
				finalErr = errors.Join(finalErr, err)
			}
		}
	}()

	// Error rendering runs before this deferred flush. AfterCompletion then sees
	// the actual transport writer and any flush error in finalErr.
	defer func() {
		if responseStage == nil {
			return
		}
		ctx.(execution.HTTPResponseContext).SetResponseWriter(responseStage.target)
		if err := responseStage.flush(); err != nil {
			finalErr = errors.Join(finalErr, fmt.Errorf("flush staged response: %w", err))
			if !responseStage.target.IsCommitted() {
				if fallbackErr := responseStage.target.WriteJSON(http.StatusInternalServerError, handler.ErrorJSON("Internal server error")); fallbackErr != nil {
					finalErr = errors.Join(finalErr, fmt.Errorf("write fallback error response: %w", fallbackErr))
				}
			}
		}
	}()

	// 실행 오류는 BeforeResponse가 끝난 뒤 HTTP 오류 응답으로 변환한다.
	defer func() {
		if finalErr == nil {
			return
		}
		if responseStage != nil {
			responseStage.rollbackSuccessResponse()
		}

		if returnedErr != nil {
			handled, err := callHandleErrorReturn(p, ctx, returnedErr)
			if err != nil {
				finalErr = errors.Join(finalErr, err)
			} else if handled {
				return
			}
		}

		if err := callHandleExecutionError(p, ctx, finalErr); err != nil {
			finalErr = errors.Join(finalErr, err)
		}
	}()

	// 패닉과 모든 조기 반환도 응답 전에 동일한 마무리 처리를 거친다.
	defer func() {
		if recovered := recover(); recovered != nil {
			finalErr = panicAsError(recovered)
		}
		if !beforeResponseCalled {
			finalErr = runBeforeResponse(interceptorCtx, globalMeta, routeFinalizers, globalFinalizers, finalErr)
			beforeResponseCalled = true
		}
	}()

	// 글로벌 인터셉터는 라우팅 전에 먼저 실행한다.
	for _, it := range p.interceptors {
		if err := it.PreHandle(interceptorCtx, globalMeta.View()); err != nil {
			if errors.Is(err, core.ErrAbortPipeline) {
				// 인터셉터가 의도적으로 요청을 종료함(응답은 이미 작성됨)
				return nil
			}
			return err
		}
		globalFinalizers = append(globalFinalizers, it)
	}

	// 라우터가 실행 대상을 결정
	meta, err := p.router.Route(ctx)
	if err != nil {
		if _, ok := ctx.(execution.HTTPResponseContext); ok {
			switch {
			case errors.Is(err, router.ErrRouteNotFound):
				return &httpfailure.HTTPError{Status: http.StatusNotFound, Message: err.Error(), Cause: err}
			case errors.Is(err, router.ErrInvalidPathEncoding):
				return &httpfailure.HTTPError{Status: http.StatusBadRequest, Message: err.Error(), Cause: err}
			}
		}
		return err
	}
	globalMeta = meta

	routeInterceptors = meta.Interceptors
	if interceptorCtx == nil && len(routeInterceptors) > 0 {
		interceptorCtx = execution.NewContext(ctx)
	}

	// 라우트 인터셉터는 요청 본문과 멀티파트를 읽기 전에 실행한다.
	// 인증되지 않은 요청이 인자 해석 비용이나 부수 효과를 유발하면 안 된다.
	for _, it := range routeInterceptors {
		if err := it.PreHandle(interceptorCtx, meta.View()); err != nil {
			if errors.Is(err, core.ErrAbortPipeline) {
				// 인터셉터가 의도적으로 요청을 종료함(응답은 이미 작성됨)
				return nil
			}
			return err
		}
		routeFinalizers = append(routeFinalizers, it)
	}

	paramMetas := resolver.HandlerParameters(meta, meta.PathKeys)

	// 인자 리졸버 체인 실행
	args, err := p.resolveArguments(ctx, paramMetas)
	if err != nil {
		return err
	}

	// 컨트롤러 메서드 호출
	results, err = p.invoker.InvokeHandler(meta, args)
	if err != nil {
		return err
	}

	returnedErr = findReturnedError(results)
	if returnedErr != nil {
		return returnedErr
	}

	if responseStage != nil {
		responseStage.checkpointSuccessResponse()
	}
	if err := p.handleSuccessReturn(ctx, results); err != nil {
		return err
	}

	// 응답이 직렬화·쿠키·status 검증을 통과한 뒤에만 domain-event 후처리를
	// 실행한다. 실제 transport flush는 BeforeResponse 이후까지 계속 지연된다.
	for _, hook := range p.postHooks {
		if err := hook.AfterExecution(ctx); err != nil {
			return err
		}
	}

	// 라우트 인터셉터의 사후 처리 실행(역순)
	for i := len(routeInterceptors) - 1; i >= 0; i-- {
		routeInterceptors[i].PostHandle(interceptorCtx, meta.View())
	}

	// 전역 인터셉터의 사후 처리 실행(역순)
	for i := len(p.interceptors) - 1; i >= 0; i-- {
		p.interceptors[i].PostHandle(interceptorCtx, meta.View())
	}

	finalErr = runBeforeResponse(interceptorCtx, globalMeta, routeFinalizers, globalFinalizers, nil)
	beforeResponseCalled = true
	if finalErr != nil {
		return finalErr
	}

	return nil
}

func findReturnedError(results []any) error {
	for _, result := range results {
		if isNilResult(result) {
			continue
		}
		if err, ok := result.(error); ok {
			return err
		}
	}
	return nil
}

func runBeforeResponse(
	ctx contextview.Ctx,
	meta execution.HandlerMeta,
	routeInterceptors []core.Interceptor,
	globalInterceptors []core.Interceptor,
	executionErr error,
) error {
	finalErr := executionErr
	for i := len(routeInterceptors) - 1; i >= 0; i-- {
		if err := callBeforeResponse(routeInterceptors[i], ctx, meta, finalErr); err != nil {
			finalErr = errors.Join(finalErr, err)
		}
	}
	for i := len(globalInterceptors) - 1; i >= 0; i-- {
		if err := callBeforeResponse(globalInterceptors[i], ctx, meta, finalErr); err != nil {
			finalErr = errors.Join(finalErr, err)
		}
	}
	return finalErr
}

func callBeforeResponse(
	interceptor core.Interceptor,
	ctx contextview.Ctx,
	meta execution.HandlerMeta,
	executionErr error,
) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = panicAsError(recovered)
		}
	}()
	return interceptor.BeforeResponse(ctx, meta.View(), executionErr)
}

func callAfterCompletion(
	interceptor core.Interceptor,
	ctx contextview.Ctx,
	meta execution.HandlerMeta,
	executionErr error,
) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = panicAsError(recovered)
		}
	}()
	interceptor.AfterCompletion(ctx, meta.View(), executionErr)
	return nil
}

func callHandleErrorReturn(
	p *Pipeline,
	ctx execution.Context,
	returnedErr error,
) (handled bool, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = panicAsError(recovered)
		}
	}()
	return p.handleErrorReturn(ctx, returnedErr)
}

func callHandleExecutionError(
	p *Pipeline,
	ctx execution.Context,
	executionErr error,
) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = panicAsError(recovered)
		}
	}()
	return p.handleExecutionError(ctx, executionErr)
}

func buildParameterMeta(method reflect.Method, pathKeys []string) []resolver.ParameterMeta {
	return resolver.Parameters(method, pathKeys)
}

// isNilResult는 명시적으로 nil을 검사합니다. error 인터페이스에 nil이 담긴 경우처럼
// 타입 정보가 있으나 값이 nil인 경우까지 포괄적으로 처리한다.
func isNilResult(v any) bool {
	if v == nil {
		return true
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.Slice, reflect.Interface:
		return rv.IsNil()
	default:
		return false
	}
}

func (p *Pipeline) handleErrorReturn(ctx execution.Context, returnedErr error) (bool, error) {
	if isNilResult(returnedErr) {
		return false, nil
	}
	resultType := reflect.TypeOf(returnedErr)
	for _, h := range p.returnHandlers {
		if h.Supports(resultType) {
			if err := h.Handle(returnedErr, ctx); err != nil {
				return false, err
			}
			return true, nil
		}
	}
	return false, fmt.Errorf("no ReturnValueHandler can handle the error return value (%s)", resultType.String())
}

func (p *Pipeline) handleSuccessReturn(ctx execution.Context, results []any) error {
	for _, result := range results {
		if isNilResult(result) {
			continue
		}
		if _, isErr := result.(error); isErr {
			continue
		}

		resultType := reflect.TypeOf(result)
		for _, h := range p.returnHandlers {
			if !h.Supports(resultType) {
				continue
			}

			if err := h.Handle(result, ctx); err != nil {
				return err
			}
			return nil
		}

		return fmt.Errorf(
			"no ReturnValueHandler is registered (%s)",
			resultType.String(),
		)
	}
	return nil
}

func (p *Pipeline) resolveArguments(ctx execution.Context, paramMetas []resolver.ParameterMeta) ([]any, error) {
	args := make([]any, 0, len(paramMetas))

	for _, paramMeta := range paramMetas {
		resolved := false

		for _, r := range p.argumentResolvers {
			if !r.Supports(paramMeta) {
				continue
			}

			val, err := r.Resolve(ctx, paramMeta)
			if err != nil {
				return nil, err
			}

			args = append(args, val)
			resolved = true
			break
		}

		if !resolved {
			return nil, fmt.Errorf(
				"no ArgumentResolver is registered for parameter %d (%s)",
				paramMeta.Index,
				paramMeta.Type.String(),
			)
		}
	}
	return args, nil
}

func (p *Pipeline) AddPostExecutionHook(hook hook.PostExecutionHook) {
	p.postHooks = append(p.postHooks, hook)
}

func (p *Pipeline) handleExecutionError(ctx execution.Context, err error) error {
	response, ok := ctx.(execution.HTTPResponseContext)
	if !ok {
		return nil
	}
	rw := response.ResponseWriter()
	if rw == nil {
		return nil
	}

	// ReturnValueHandler 등에서 이미 응답이 커밋된 경우 이중 응답을 방지한다.
	if rw.IsCommitted() {
		return nil
	}

	status := http.StatusInternalServerError
	message := "Internal server error"
	var responseErr error
	var body json.RawMessage
	var httpErr *httpfailure.HTTPError
	if errors.As(err, &httpErr) {
		if httpErr.Status < 400 || httpErr.Status > 599 {
			responseErr = fmt.Errorf("invalid HTTP error status %d", httpErr.Status)
		} else {
			status = httpErr.Status
			message = httpErr.Message
		}
	} else if validationBody, ok := handler.ValidationErrorJSON(err); ok {
		status = http.StatusBadRequest
		body = validationBody
	} else {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			status = http.StatusRequestEntityTooLarge
			message = "Request entity too large"
		}
	}

	if body == nil {
		body = handler.InputErrorJSON(err, status, message)
	}
	if writeErr := rw.WriteJSON(status, body); writeErr != nil {
		responseErr = errors.Join(responseErr, fmt.Errorf("write execution error response: %w", writeErr))
	}
	return responseErr
}

func stageResponseWriter(ctx execution.Context) *stagedResponseWriter {
	response, ok := ctx.(execution.HTTPResponseContext)
	if !ok {
		return nil
	}
	rw := response.ResponseWriter()
	if rw == nil {
		return nil
	}
	staged := newStagedResponseWriter(rw)
	response.SetResponseWriter(staged)
	return staged
}

func panicAsError(recovered any) error {
	if err, ok := recovered.(error); ok {
		return fmt.Errorf("panic recovered: %w\n%s", err, debug.Stack())
	}
	return fmt.Errorf("panic recovered: %v\n%s", recovered, debug.Stack())
}
