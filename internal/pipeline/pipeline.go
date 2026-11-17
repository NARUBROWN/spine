package pipeline

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/contextview"
	"github.com/NARUBROWN/spine/internal/event/hook"
	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/handler"
	"github.com/NARUBROWN/spine/internal/httpfailure"
	"github.com/NARUBROWN/spine/internal/invoker"
	"github.com/NARUBROWN/spine/internal/pipelinecontrol"
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
func (p *Pipeline) Execute(ctx execution.Context, input *execution.Input) error {
	return p.execute(ctx, func() (execution.HandlerMeta, error) { return p.router.Route(input.Routing()) }, input)
}

// ExecuteRegistered는 이미 전송 계층에서 선택한 핸들러를 실행합니다.
func (p *Pipeline) ExecuteRegistered(ctx execution.Context, meta execution.HandlerMeta, input *execution.Input) error {
	return p.execute(ctx, func() (execution.HandlerMeta, error) {
		return meta, nil
	}, input)
}

func (p *Pipeline) execute(ctx execution.Context, resolve func() (execution.HandlerMeta, error), input *execution.Input) (finalErr error) {
	// Detach transport data after completion hooks and before runtime settlement.
	defer input.Close()
	// Release per-execution values after completion.
	if store, ok := ctx.(interface{ ReleaseStore() }); ok {
		defer store.ReleaseStore()
	}
	globalMeta := execution.HandlerMeta{}
	var interceptorCtx contextview.Ctx
	if len(p.interceptors) > 0 {
		interceptorCtx = execution.NewContext(ctx, input)
	}
	var routeInterceptors []core.Interceptor
	var globalFinalizers []core.Interceptor
	var routeFinalizers []core.Interceptor
	var results []any
	var returnedErr error
	var interceptorErr error
	beforeResponseCalled := false
	responseStage := stageResponseWriter(input)

	// AfterCompletion은 응답 처리가 끝난 뒤 실행한다.
	defer func() {
		finalErr = execution.AfterCompletion(interceptorCtx, globalMeta, routeInterceptors, finalErr)
		finalErr = execution.AfterCompletion(interceptorCtx, globalMeta, p.interceptors, finalErr)
	}()

	// Error rendering runs before this deferred flush. AfterCompletion then sees
	// the actual transport writer and any flush error in finalErr.
	defer func() {
		if responseStage == nil {
			return
		}
		input.HTTPResponse().SetResponseWriter(responseStage.target)
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
			if interceptorErr != nil {
				responseStage.prepared = false
			}
		}

		if returnedErr != nil && interceptorErr == nil {
			handled, err := callHandleErrorReturn(p, ctx, input, returnedErr)
			if err != nil {
				finalErr = errors.Join(finalErr, err)
			} else if handled {
				return
			}
		}

		responseErr := finalErr
		if interceptorErr != nil {
			responseErr = interceptorErr
		}
		if err := callHandleExecutionError(p, ctx, input, responseErr); err != nil {
			finalErr = errors.Join(finalErr, err)
		}
	}()

	// 패닉과 모든 조기 반환도 응답 전에 동일한 마무리 처리를 거친다.
	defer func() {
		if recovered := recover(); recovered != nil {
			finalErr = execution.PanicError(recovered)
		}
		if !beforeResponseCalled {
			var beforeErr error
			finalErr, beforeErr = runBeforeResponse(interceptorCtx, globalMeta, routeFinalizers, globalFinalizers, finalErr)
			if beforeErr != nil {
				interceptorErr = beforeErr
			}
			beforeResponseCalled = true
		}
	}()

	// 글로벌 인터셉터는 라우팅 전에 먼저 실행한다.
	for _, it := range p.interceptors {
		if err := it.PreHandle(interceptorCtx, globalMeta.View()); err != nil {
			if errors.Is(err, pipelinecontrol.ErrAbort) {
				// 인터셉터가 의도적으로 요청을 종료함(응답은 이미 작성됨)
				return nil
			}
			interceptorErr = err
			return err
		}
		globalFinalizers = append(globalFinalizers, it)
	}

	// 라우터가 실행 대상을 결정
	meta, err := resolve()
	if err != nil {
		if input.HTTPResponse() != nil {
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
		interceptorCtx = execution.NewContext(ctx, input)
	}

	// 라우트 인터셉터는 요청 본문과 멀티파트를 읽기 전에 실행한다.
	// 인증되지 않은 요청이 인자 해석 비용이나 부수 효과를 유발하면 안 된다.
	for _, it := range routeInterceptors {
		if err := it.PreHandle(interceptorCtx, meta.View()); err != nil {
			if errors.Is(err, pipelinecontrol.ErrAbort) {
				// 인터셉터가 의도적으로 요청을 종료함(응답은 이미 작성됨)
				return nil
			}
			interceptorErr = err
			return err
		}
		routeFinalizers = append(routeFinalizers, it)
	}

	paramMetas := resolver.HandlerParameters(meta, meta.PathKeys)

	// 인자 리졸버 체인 실행
	args, err := p.resolveArguments(ctx, input, paramMetas)
	if err != nil {
		return err
	}

	// 컨트롤러 메서드 호출
	results, err = p.invoker.InvokeHandler(meta, args)
	if err != nil {
		return err
	}

	returnedErr = invoker.ReturnedError(results)
	if returnedErr != nil {
		return returnedErr
	}

	if responseStage != nil {
		responseStage.checkpointSuccessResponse()
	}
	if err := p.handleSuccessReturn(ctx, input, results); err != nil {
		return err
	}

	// Run general post-execution hooks after response preparation.
	// The transport flush remains deferred until after BeforeResponse.
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

	finalErr, interceptorErr = runBeforeResponse(interceptorCtx, globalMeta, routeFinalizers, globalFinalizers, nil)
	beforeResponseCalled = true
	if finalErr != nil {
		return finalErr
	}

	return nil
}

func runBeforeResponse(
	ctx contextview.Ctx,
	meta execution.HandlerMeta,
	routeInterceptors []core.Interceptor,
	globalInterceptors []core.Interceptor,
	executionErr error,
) (error, error) {
	finalErr, routeErr := execution.BeforeResponse(ctx, meta, routeInterceptors, executionErr)
	finalErr, globalErr := execution.BeforeResponse(ctx, meta, globalInterceptors, finalErr)
	if globalErr != nil {
		return finalErr, globalErr
	}
	return finalErr, routeErr
}

func callHandleErrorReturn(
	p *Pipeline,
	ctx execution.Context,
	input *execution.Input,
	returnedErr error,
) (handled bool, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = execution.PanicError(recovered)
		}
	}()
	return p.handleErrorReturn(ctx, input, returnedErr)
}

func callHandleExecutionError(
	p *Pipeline,
	ctx execution.Context,
	input *execution.Input,
	executionErr error,
) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = execution.PanicError(recovered)
		}
	}()
	return p.handleExecutionError(ctx, input, executionErr)
}

func buildParameterMeta(method reflect.Method, pathKeys []string) []resolver.ParameterMeta {
	return resolver.Parameters(method, pathKeys)
}

func (p *Pipeline) handleErrorReturn(ctx execution.Context, input *execution.Input, returnedErr error) (bool, error) {
	if invoker.IsNilResult(returnedErr) {
		return false, nil
	}
	resultType := reflect.TypeOf(returnedErr)
	for _, h := range p.returnHandlers {
		if h.Supports(resultType) {
			if err := h.Handle(returnedErr, ctx, input); err != nil {
				return false, err
			}
			return true, nil
		}
	}
	return false, fmt.Errorf("no ReturnValueHandler can handle the error return value (%s)", resultType.String())
}

func (p *Pipeline) handleSuccessReturn(ctx execution.Context, input *execution.Input, results []any) error {
	for _, result := range results {
		if invoker.IsNilResult(result) {
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

			if err := h.Handle(result, ctx, input); err != nil {
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

func (p *Pipeline) resolveArguments(ctx execution.Context, input *execution.Input, paramMetas []resolver.ParameterMeta) ([]any, error) {
	args := make([]any, 0, len(paramMetas))

	for _, paramMeta := range paramMetas {
		resolved := false

		for _, r := range p.argumentResolvers {
			if !r.Supports(paramMeta) {
				continue
			}

			val, err := r.Resolve(ctx, paramMeta, input)
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

func (p *Pipeline) handleExecutionError(ctx execution.Context, input *execution.Input, err error) error {
	response := input.HTTPResponse()
	ok := response != nil
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

	return handler.WriteErrorResponse(rw, err)
}

func stageResponseWriter(input *execution.Input) *stagedResponseWriter {
	response := input.HTTPResponse()
	ok := response != nil
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
