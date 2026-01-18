package pipeline

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/handler"
	"github.com/NARUBROWN/spine/internal/invoker"
	"github.com/NARUBROWN/spine/internal/resolver"
	"github.com/NARUBROWN/spine/internal/router"
	"github.com/NARUBROWN/spine/pkg/path"
)

type Pipeline struct {
	router            router.Router
	interceptors      []core.Interceptor
	argumentResolvers []resolver.ArgumentResolver
	returnHandlers    []handler.ReturnValueHandler
	invoker           *invoker.Invoker
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
	// Router가 실행 대상을 결정
	meta, err := p.router.Route(ctx)

	if err != nil {
		return err
	}

	paramMetas := buildParameterMeta(meta.Method, ctx)

	// Argument Resolver 체인 실행
	args, err := p.resolveArguments(ctx, paramMetas)
	if err != nil {
		return err
	}

	// Interceptor preHandle
	for _, it := range p.interceptors {
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
	if err := p.handleReturn(ctx, results); err != nil {
		return err
	}

	// Interceptor postHandle (역순)
	for i := len(p.interceptors) - 1; i >= 0; i-- {
		p.interceptors[i].PostHandle(ctx, meta)
	}

	// Interceptor AfterCompletion은 무조건 보장
	defer func() {
		for i := len(p.interceptors) - 1; i >= 0; i-- {
			p.interceptors[i].AfterCompletion(ctx, meta, finalErr)
		}
	}()

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

func (p *Pipeline) handleReturn(ctx core.ExecutionContext, results []any) error {
	// error가 있으면 error만 처리하고 종료
	for _, result := range results {
		if result == nil {
			continue
		}
		if _, isErr := result.(error); isErr {
			resultType := reflect.TypeOf(result)
			for _, h := range p.returnHandlers {
				if h.Supports(resultType) {
					return h.Handle(result, ctx)
				}
			}
		}
	}

	// error가 없으면 척번째 non-nil 값 처리
	for _, result := range results {
		if result == nil {
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
	reqCtx, ok := ctx.(core.RequestContext)
	if !ok {
		return nil, fmt.Errorf("ExecutionContext이 RequestContext를 구현하고 있지 않습니다.")
	}

	args := make([]any, 0, len(paramMetas))

	for _, paramMeta := range paramMetas {
		resolved := false

		for _, r := range p.argumentResolvers {
			if !r.Supports(paramMeta) {
				continue
			}

			val, err := r.Resolve(reqCtx, paramMeta)
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
