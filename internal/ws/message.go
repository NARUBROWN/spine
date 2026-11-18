package ws

import (
	"context"
	"errors"
	"fmt"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/invoker"
	"github.com/NARUBROWN/spine/internal/pipelinecontrol"
	"github.com/NARUBROWN/spine/internal/resolver"
	wsresolver "github.com/NARUBROWN/spine/internal/ws/resolver"
	"github.com/NARUBROWN/spine/internal/ws/sendctx"
	"github.com/NARUBROWN/spine/pkg/spine"
)

type messageContext struct {
	context.Context
	values execution.Store
}

func (ctx *messageContext) Set(key string, value any)  { ctx.values.Set(key, value) }
func (ctx *messageContext) Get(key string) (any, bool) { return ctx.values.Get(key) }

// executeMessage는 프레임별 인터셉터, 핸들러와 컨텍스트 수명을 관리합니다.
func (r *Runtime) executeMessage(connection session, kind int, payload []byte, send func(int, []byte) error, meta execution.HandlerMeta, routes ...core.WebSocketInterceptor) (finalErr error) {
	ctx, cancel := context.WithCancel(connection.ctx)
	defer cancel()
	ctx, release := sendctx.Bind(ctx, send)
	defer release()
	messageCtx := &messageContext{Context: ctx}
	defer messageCtx.values.ReleaseStore()

	var activeMeta execution.HandlerMeta
	var activeRoutes []core.WebSocketInterceptor
	var returnedErr error
	var globalStarted, routeStarted int
	defer func() {
		if recovered := recover(); recovered != nil {
			finalErr = execution.PanicError(recovered)
		}
		var routeBeforeErr, globalBeforeErr error
		finalErr, routeBeforeErr = beforeMessageCompletion(messageCtx, activeMeta.View(), activeRoutes[:routeStarted], finalErr)
		finalErr, globalBeforeErr = beforeMessageCompletion(messageCtx, activeMeta.View(), r.interceptors[:globalStarted], finalErr)
		if returnedErr != nil && routeBeforeErr == nil && globalBeforeErr == nil {
			// 완료 훅이 받는 기존 반환 오류의 구성을 유지합니다.
			finalErr = errors.Join(finalErr, fmt.Errorf("no ReturnValueHandler can handle the error return value (%T)", returnedErr))
		}
		finalErr = afterMessageCompletion(messageCtx, activeMeta.View(), activeRoutes, finalErr)
		finalErr = afterMessageCompletion(messageCtx, activeMeta.View(), r.interceptors, finalErr)
	}()

	for _, interceptor := range r.interceptors {
		if err := interceptor.PreMessage(messageCtx, activeMeta.View()); err != nil {
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
		if err := interceptor.PreMessage(messageCtx, activeMeta.View()); err != nil {
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
		activeRoutes[i].PostMessage(messageCtx, activeMeta.View())
	}
	for i := len(r.interceptors) - 1; i >= 0; i-- {
		r.interceptors[i].PostMessage(messageCtx, activeMeta.View())
	}
	return nil
}

func beforeMessageCompletion(ctx spine.WebSocketCtx, meta core.HandlerMeta, interceptors []core.WebSocketInterceptor, finalErr error) (error, error) {
	var lastErr error
	for i := len(interceptors) - 1; i >= 0; i-- {
		if err := callMessageHook(func() error {
			return interceptors[i].BeforeMessageCompletion(ctx, meta, finalErr)
		}); err != nil {
			finalErr = errors.Join(finalErr, err)
			lastErr = err
		}
	}
	return finalErr, lastErr
}

func afterMessageCompletion(ctx spine.WebSocketCtx, meta core.HandlerMeta, interceptors []core.WebSocketInterceptor, finalErr error) error {
	for i := len(interceptors) - 1; i >= 0; i-- {
		if err := callMessageHook(func() error {
			interceptors[i].AfterMessageCompletion(ctx, meta, finalErr)
			return nil
		}); err != nil {
			finalErr = errors.Join(finalErr, err)
		}
	}
	return finalErr
}

func callMessageHook(hook func() error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = execution.PanicError(recovered)
		}
	}()
	return hook()
}
