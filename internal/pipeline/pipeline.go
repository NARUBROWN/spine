package pipeline

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/event/hook"
	"github.com/NARUBROWN/spine/internal/handler"
	"github.com/NARUBROWN/spine/internal/invoker"
	"github.com/NARUBROWN/spine/internal/resolver"
	"github.com/NARUBROWN/spine/internal/router"
	"github.com/NARUBROWN/spine/pkg/httperr"
	"github.com/NARUBROWN/spine/pkg/path"
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
func (p *Pipeline) Execute(ctx core.ExecutionContext) (finalErr error) {
	defer func() {
		if finalErr != nil {
			p.handleExecutionError(ctx, finalErr)
		}
	}()

	// Router가 실행 대상을 결정
	meta, err := p.router.Route(ctx)

	if err != nil {
		return err
	}

	interceptors := p.composeInterceptors(meta)

	// Interceptor AfterCompletion은 무조건 보장
	defer func() {
		for i := len(interceptors) - 1; i >= 0; i-- {
			interceptors[i].AfterCompletion(ctx, meta, finalErr)
		}
	}()

	paramMetas := buildParameterMeta(meta.Method, ctx)

	// Argument Resolver 체인 실행
	args, err := p.resolveArguments(ctx, paramMetas)
	if err != nil {
		return err
	}

	// Interceptor preHandle
	for _, it := range interceptors {
		if err := it.PreHandle(ctx, meta); err != nil {
			if errors.Is(err, core.ErrAbortPipeline) {
				// Interceptor가 의도적으로 요청을 종료함 (응답은 이미 작성됨)
				return nil
			}
			return err
		}
	}

	// Controller Method 호출
	results, err := p.invoker.Invoke(
		meta.ControllerType,
		meta.Method,
		args,
	)
	if err != nil {
		return err
	}

	// ReturnValueHandler 처리
	returnError := p.handleReturn(ctx, results)

	// PostHooks 추가
	for _, hook := range p.postHooks {
		hook.AfterExecution(ctx, results, returnError)
	}

	if returnError != nil {
		return returnError
	}

	// Interceptor postHandle (역순)
	for i := len(interceptors) - 1; i >= 0; i-- {
		interceptors[i].PostHandle(ctx, meta)
	}

	return nil
}

func (p *Pipeline) composeInterceptors(meta core.HandlerMeta) []core.Interceptor {
	total := make([]core.Interceptor, 0, len(p.interceptors)+len(meta.Interceptors))

	/*
		실행 순서 정책
		1. 전역 Interceptor를 먼저 실행
		2. 이후 라우트(Handler)에 바인딩된 Interceptor를 실행
		3. PostHandle / AfterCompletion은 이 순서의 역순으로 실행됨
	*/
	total = append(total, p.interceptors...)    // 전역 인터셉터
	total = append(total, meta.Interceptors...) // 라우트 인터셉터

	return total
}

func buildParameterMeta(method reflect.Method, ctx core.ExecutionContext) []resolver.ParameterMeta {

	pathKeys := ctx.PathKeys() // ["id"]

	pathIdx := 0
	var metas []resolver.ParameterMeta

	for i := 1; i < method.Type.NumIn(); i++ {
		pt := method.Type.In(i)

		pm := resolver.ParameterMeta{
			Index: i - 1,
			Type:  pt,
		}

		if isPathType(pt) {
			if pathIdx >= len(pathKeys) {
				pm.PathKey = ""
			} else {
				pm.PathKey = pathKeys[pathIdx]
			}
			pathIdx++
		}

		metas = append(metas, pm)
	}

	return metas
}

func isPathType(pt reflect.Type) bool {
	pathPkg := reflect.TypeFor[path.Int]().PkgPath()
	return pt.PkgPath() == pathPkg
}

// isNilResult 명시적 nil 체크: error 인터페이스에 nil이 담긴 경우처럼
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

func (p *Pipeline) handleReturn(ctx core.ExecutionContext, results []any) error {
	// error가 있으면 error만 처리하고 종료
	for _, result := range results {
		if isNilResult(result) {
			continue
		}
		if _, isErr := result.(error); isErr {
			resultType := reflect.TypeOf(result)
			for _, h := range p.returnHandlers {
				if h.Supports(resultType) {
					if err := h.Handle(result, ctx); err != nil {
						return err
					}
					// error 반환값은 여기서 소비하고 종료한다.
					return nil
				}
			}
			return fmt.Errorf(
				"error 반환값을 처리할 ReturnValueHandler가 없습니다. (%s)",
				resultType.String(),
			)
		}
	}

	// error가 없으면 척번째 non-nil 값 처리
	for _, result := range results {
		if isNilResult(result) {
			continue
		}

		resultType := reflect.TypeOf(result)
		handled := false

		for _, h := range p.returnHandlers {
			if !h.Supports(resultType) {
				continue
			}

			if err := h.Handle(result, ctx); err != nil {
				return err
			}

			handled = true
			break
		}

		if !handled {
			return fmt.Errorf(
				"ReturnValueHandler가 없습니다. (%s)",
				resultType.String(),
			)
		}
	}
	return nil
}

func (p *Pipeline) resolveArguments(ctx core.ExecutionContext, paramMetas []resolver.ParameterMeta) ([]any, error) {
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
				"ArgumentResolver에 parameter가 없습니다. %d (%s)",
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

func (p *Pipeline) handleExecutionError(ctx core.ExecutionContext, err error) {
	rwAny, ok := ctx.Get("spine.response_writer")
	if !ok {
		return
	}

	rw, ok := rwAny.(core.ResponseWriter)
	if !ok {
		return
	}

	// ReturnValueHandler 등에서 이미 응답이 커밋된 경우 이중 응답을 방지한다.
	if rw.IsCommitted() {
		return
	}

	var httpErr *httperr.HTTPError
	if errors.As(err, &httpErr) {
		rw.WriteJSON(
			httpErr.Status,
			map[string]any{
				"message": httpErr.Message,
			},
		)
		return
	}

	rw.WriteJSON(
		500,
		map[string]any{
			"message": "Internal server error",
		},
	)
}
