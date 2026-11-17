package ws

import (
	"context"
	"errors"
	"fmt"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/contextview"
	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/invoker"
	"github.com/NARUBROWN/spine/internal/pipelinecontrol"
	"github.com/NARUBROWN/spine/internal/resolver"
	wsresolver "github.com/NARUBROWN/spine/internal/ws/resolver"
	"github.com/NARUBROWN/spine/internal/ws/sendctx"
)

// executeMessage는 프레임별 인터셉터, 핸들러와 컨텍스트 수명을 관리합니다.
func (r *Runtime) executeMessage(connection session, kind int, payload []byte, send func(int, []byte) error, meta execution.HandlerMeta) (finalErr error) {
	messageCtx, cancel := context.WithCancel(connection.ctx)
	defer cancel()
	messageCtx, release := sendctx.Bind(messageCtx, send)
	defer release()

	state := execution.NewState(messageCtx)
	defer state.ReleaseStore()
	routes := meta.Interceptors
	var view contextview.Ctx
	if len(r.interceptors) != 0 || len(routes) != 0 {
		view = execution.NewContext(state, nil)
	}

	var activeMeta execution.HandlerMeta
	var activeRoutes []core.Interceptor
	var returnedErr error
	var globalStarted, routeStarted int
	defer func() {
		if recovered := recover(); recovered != nil {
			finalErr = execution.PanicError(recovered)
		}
		var routeBeforeErr, globalBeforeErr error
		finalErr, routeBeforeErr = execution.BeforeResponse(view, activeMeta, activeRoutes[:routeStarted], finalErr)
		finalErr, globalBeforeErr = execution.BeforeResponse(view, activeMeta, r.interceptors[:globalStarted], finalErr)
		if returnedErr != nil && routeBeforeErr == nil && globalBeforeErr == nil {
			// 완료 훅이 받는 기존 반환 오류의 구성을 유지합니다.
			finalErr = errors.Join(finalErr, fmt.Errorf("no ReturnValueHandler can handle the error return value (%T)", returnedErr))
		}
		finalErr = execution.AfterCompletion(view, activeMeta, activeRoutes, finalErr)
		finalErr = execution.AfterCompletion(view, activeMeta, r.interceptors, finalErr)
	}()

	for _, interceptor := range r.interceptors {
		if err := interceptor.PreHandle(view, activeMeta.View()); err != nil {
			if errors.Is(err, pipelinecontrol.ErrAbort) {
				return nil
			}
			return err
		}
		globalStarted++
	}

	activeMeta = meta
	activeRoutes = routes
	for _, interceptor := range activeRoutes {
		if err := interceptor.PreHandle(view, activeMeta.View()); err != nil {
			if errors.Is(err, pipelinecontrol.ErrAbort) {
				return nil
			}
			return err
		}
		routeStarted++
	}

	input := &wsresolver.Input{ConnectionID: connection.id, Kind: kind, Payload: payload}
	args, err := wsresolver.Bind(messageCtx, resolver.HandlerParameters(meta, meta.PathKeys), input, r.argumentResolvers)
	if err != nil {
		return err
	}
	results, err := r.invoker.InvokeHandler(meta, args)
	if err != nil {
		return err
	}
	returnedErr = invoker.ReturnedError(results)
	if returnedErr != nil {
		return returnedErr
	}
	for _, result := range results {
		if !invoker.IsNilResult(result) {
			return fmt.Errorf("no ReturnValueHandler is registered (%T)", result)
		}
	}

	for i := len(activeRoutes) - 1; i >= 0; i-- {
		activeRoutes[i].PostHandle(view, activeMeta.View())
	}
	for i := len(r.interceptors) - 1; i >= 0; i-- {
		r.interceptors[i].PostHandle(view, activeMeta.View())
	}
	return nil
}
