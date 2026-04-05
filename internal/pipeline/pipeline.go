package pipeline

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"runtime/debug"

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
		if recovered := recover(); recovered != nil {
			finalErr = panicAsError(recovered)
		}
		if finalErr != nil {
			p.handleExecutionError(ctx, finalErr)
		}
	}()

	globalMeta := core.HandlerMeta{}
	defer func() {
		for i := len(p.interceptors) - 1; i >= 0; i-- {
			p.interceptors[i].AfterCompletion(ctx, globalMeta, finalErr)
		}
	}()

	// 글로벌 Interceptor는 라우팅 전에 먼저 실행한다.
	for _, it := range p.interceptors {
		if err := it.PreHandle(ctx, globalMeta); err != nil {
			if errors.Is(err, core.ErrAbortPipeline) {
				// Interceptor가 의도적으로 요청을 종료함 (응답은 이미 작성됨)
				return nil
			}
			return err
		}
	}

	// Router가 실행 대상을 결정
	meta, err := p.router.Route(ctx)
	if err != nil {
		return err
	}

	routeInterceptors := meta.Interceptors

	// 라우트 Interceptor AfterCompletion은 무조건 보장
	defer func() {
		for i := len(routeInterceptors) - 1; i >= 0; i-- {
			routeInterceptors[i].AfterCompletion(ctx, meta, finalErr)
		}
	}()

	paramMetas := buildParameterMeta(meta.Method, ctx)

	// Argument Resolver 체인 실행
	args, err := p.resolveArguments(ctx, paramMetas)
	if err != nil {
		return err
	}

	// 라우트 Interceptor preHandle
	for _, it := range routeInterceptors {
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

	handled, err := p.handleErrorReturn(ctx, results)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}

	for _, hook := range p.postHooks {
		if err := hook.AfterExecution(ctx, results, nil); err != nil {
			return err
		}
	}

	if err := p.handleSuccessReturn(ctx, results); err != nil {
		return err
	}

	// 라우트 Interceptor postHandle (역순)
	for i := len(routeInterceptors) - 1; i >= 0; i-- {
		routeInterceptors[i].PostHandle(ctx, meta)
	}

	// 글로벌 Interceptor postHandle (역순)
	for i := len(p.interceptors) - 1; i >= 0; i-- {
		p.interceptors[i].PostHandle(ctx, meta)
	}

	return nil
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

func (p *Pipeline) handleErrorReturn(ctx core.ExecutionContext, results []any) (bool, error) {
	for _, result := range results {
		if isNilResult(result) {
			continue
		}
		if _, isErr := result.(error); isErr {
			resultType := reflect.TypeOf(result)
			for _, h := range p.returnHandlers {
				if h.Supports(resultType) {
					if err := h.Handle(result, ctx); err != nil {
						return false, err
					}
					return true, nil
				}
			}
			return false, fmt.Errorf(
				"error 반환값을 처리할 ReturnValueHandler가 없습니다. (%s)",
				resultType.String(),
			)
		}
	}

	return false, nil
}

func (p *Pipeline) handleSuccessReturn(ctx core.ExecutionContext, results []any) error {
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
			"ReturnValueHandler가 없습니다. (%s)",
			resultType.String(),
		)
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

	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		rw.WriteJSON(
			http.StatusRequestEntityTooLarge,
			map[string]any{
				"message": "Request entity too large",
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

func panicAsError(recovered any) error {
	if err, ok := recovered.(error); ok {
		return fmt.Errorf("panic recovered: %w\n%s", err, debug.Stack())
	}
	return fmt.Errorf("panic recovered: %v\n%s", recovered, debug.Stack())
}
