package pipeline

import (
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
	router router.Router
	// interceptors      []spine.Interceptor
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

func (p *Pipeline) AddArgumentResolver(resolvers ...resolver.ArgumentResolver) {
	p.argumentResolvers = append(p.argumentResolvers, resolvers...)
}

func (p *Pipeline) AddReturnValueHandler(handlers ...handler.ReturnValueHandler) {
	p.returnHandlers = append(p.returnHandlers, handlers...)
}

// Execute는 하나의 요청 실행 전체를 소유합니다.
func (p *Pipeline) Execute(ctx core.ExecutionContext) error {
	// Router가 실행 대상을 결정
	meta, err := p.router.Route(ctx)

	if err != nil {
		return err
	}

	paramMetas := buildParameterMeta(meta.Method, ctx)

	// Argument Resolver 체인 실행
	args, err := p.resolveArguments(ctx, meta, paramMetas)
	if err != nil {
		return err
	}

	// TODO: Interceptor preHandle

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
	if err := p.handleReturn(ctx, meta, results); err != nil {
		return err
	}

	// TODO: Interceptor postHandle (역순)

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
	pathPkg := reflect.TypeOf(path.Int{}).PkgPath()
	return pt.PkgPath() == pathPkg
}

func (p *Pipeline) handleReturn(ctx core.ExecutionContext, meta router.HandlerMeta, results []any) error {
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

func (p *Pipeline) resolveArguments(ctx core.ExecutionContext, meta router.HandlerMeta, paramMetas []resolver.ParameterMeta) ([]any, error) {
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
