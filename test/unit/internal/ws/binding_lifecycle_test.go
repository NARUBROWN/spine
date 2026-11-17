package ws

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/container"
	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/invoker"
	"github.com/NARUBROWN/spine/internal/pipelinecontrol"
	wsresolver "github.com/NARUBROWN/spine/internal/ws/resolver"
	"github.com/NARUBROWN/spine/pkg/input"
	publicws "github.com/NARUBROWN/spine/pkg/ws"
)

type retainedExecutionObserver struct{ retained publicws.MessageContext }

func (o *retainedExecutionObserver) PreMessage(ctx publicws.MessageContext, _ core.HandlerMeta) error {
	o.retained = ctx
	ctx.Set("owned", true)
	return nil
}
func (*retainedExecutionObserver) PostMessage(publicws.MessageContext, core.HandlerMeta) {}
func (*retainedExecutionObserver) BeforeMessageCompletion(publicws.MessageContext, core.HandlerMeta, error) error {
	return nil
}
func (*retainedExecutionObserver) AfterMessageCompletion(publicws.MessageContext, core.HandlerMeta, error) {
}

func TestMessageRuntimeReleasesOwnedScopeForEveryOutcome(t *testing.T) {
	for _, outcome := range []string{"success", "error", "panic", "invalid-input"} {
		t.Run(outcome, func(t *testing.T) {
			observer := &retainedExecutionObserver{}
			runtime := &Runtime{invoker: invoker.NewInvoker(container.New()), interceptors: []core.WebSocketInterceptor{observer}, argumentResolvers: wsresolver.Resolvers(nil)}
			connectionCtx, cancel := context.WithCancel(t.Context())
			defer cancel()
			callback := func(ctx context.Context, message input.Message[struct{ Text string }]) error {
				if ctx != observer.retained {
					t.Error("핸들러가 인터셉터와 같은 프레임 컨텍스트를 받지 못했습니다")
				}
				switch outcome {
				case "error":
					return errors.New("failed")
				case "panic":
					panic("failed")
				}
				return nil
			}
			payload := []byte(`{"Text":"hello"}`)
			if outcome == "invalid-input" {
				payload = []byte(`{`)
			}
			err := runtime.executeMessage(session{ctx: connectionCtx, id: "connection"}, 1, payload, func(int, []byte) error { return nil }, execution.HandlerMeta{Callback: reflect.ValueOf(callback)})
			if (err != nil) != (outcome != "success") {
				t.Fatal(err)
			}
			if observer.retained.Err() != context.Canceled {
				t.Fatal("message context remains active")
			}
			if _, ok := observer.retained.Get("owned"); ok {
				t.Fatal("message store retained")
			}
			if err := publicws.Send(observer.retained, 1, nil); !errors.Is(err, publicws.ErrConnectionNotFound) {
				t.Fatal("compatibility sender retained", err)
			}
			if connectionCtx.Err() != nil {
				t.Fatal("message completion canceled connection")
			}
		})
	}
}

var retainedMessageContexts []context.Context

func BenchmarkRuntimeCompletedMessageChurn(b *testing.B) {
	retainedMessageContexts = make([]context.Context, 256)
	next := 0
	callback := func(ctx context.Context, _ input.Message[[]byte]) error {
		retainedMessageContexts[next%len(retainedMessageContexts)] = ctx
		next++
		return nil
	}
	meta := execution.HandlerMeta{Callback: reflect.ValueOf(callback)}
	runtime := &Runtime{invoker: invoker.NewInvoker(container.New()), argumentResolvers: wsresolver.Resolvers(nil)}
	connection := session{ctx: context.Background(), id: "connection"}
	for n := 0; n < b.N; n++ {
		if err := runtime.executeMessage(connection, 2, make([]byte, 16<<10), nil, meta); err != nil {
			b.Fatal(err)
		}
	}
}

type frameInterceptor struct {
	name            string
	log             *[]string
	preErr          error
	beforeErr       error
	completionPanic error
	receivedErr     error
}

func (i *frameInterceptor) PreMessage(ctx publicws.MessageContext, _ core.HandlerMeta) error {
	*i.log = append(*i.log, i.name+" pre")
	ctx.Set(i.name, true)
	return i.preErr
}

func (i *frameInterceptor) PostMessage(publicws.MessageContext, core.HandlerMeta) {
	*i.log = append(*i.log, i.name+" post")
}

func (i *frameInterceptor) BeforeMessageCompletion(publicws.MessageContext, core.HandlerMeta, error) error {
	*i.log = append(*i.log, i.name+" before")
	return i.beforeErr
}

func (i *frameInterceptor) AfterMessageCompletion(ctx publicws.MessageContext, _ core.HandlerMeta, err error) {
	*i.log = append(*i.log, i.name+" complete")
	i.receivedErr = err
	if ctx.Err() != nil {
		panic("완료 훅 실행 중 프레임 컨텍스트가 취소됐습니다")
	}
	if i.completionPanic != nil {
		panic(i.completionPanic)
	}
}

func TestRuntimeExecutesFrameInterceptorsInOrder(t *testing.T) {
	rejected := errors.New("요청 거부")
	handlerPanic := errors.New("핸들러 패닉")
	handlerErr := errors.New("핸들러 오류")
	beforeErr := errors.New("응답 전 훅 실패")
	completionPanic := errors.New("완료 훅 패닉")
	for _, tt := range []struct {
		name                                                                            string
		payload                                                                         string
		globalPreErr, routePreErr, panicInHandler, routeBeforeErr, routeCompletionPanic error
		handlerErr                                                                      error
		wantErr                                                                         error
		wantDecodeError                                                                 bool
		want                                                                            []string
	}{
		{
			name: "성공", payload: `{"Text":"ok"}`,
			want: []string{"global pre", "route pre", "handler", "route post", "global post", "route before", "global before", "route complete", "global complete"},
		},
		{
			name: "라우트에서 거부하면 디코딩 생략", payload: "{", routePreErr: rejected, wantErr: rejected,
			want: []string{"global pre", "route pre", "global before", "route complete", "global complete"},
		},
		{
			name: "전역 중단", payload: "{", globalPreErr: pipelinecontrol.ErrAbort,
			want: []string{"global pre", "global complete"},
		},
		{
			name: "디코딩 실패", payload: "{", wantDecodeError: true,
			want: []string{"global pre", "route pre", "route before", "global before", "route complete", "global complete"},
		},
		{
			name: "핸들러 오류", payload: `{"Text":"ok"}`, handlerErr: handlerErr, wantErr: handlerErr,
			want: []string{"global pre", "route pre", "handler", "route before", "global before", "route complete", "global complete"},
		},
		{
			name: "핸들러 패닉", payload: `{"Text":"ok"}`, panicInHandler: handlerPanic, wantErr: handlerPanic,
			want: []string{"global pre", "route pre", "handler", "route before", "global before", "route complete", "global complete"},
		},
		{
			name: "마무리 훅 오류 누적", payload: `{"Text":"ok"}`, routeBeforeErr: beforeErr, routeCompletionPanic: completionPanic, wantErr: beforeErr,
			want: []string{"global pre", "route pre", "handler", "route post", "global post", "route before", "global before", "route complete", "global complete"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var log []string
			global := &frameInterceptor{name: "global", log: &log, preErr: tt.globalPreErr}
			route := &frameInterceptor{name: "route", log: &log, preErr: tt.routePreErr, beforeErr: tt.routeBeforeErr, completionPanic: tt.routeCompletionPanic}
			runtime := &Runtime{
				invoker:           invoker.NewInvoker(container.New()),
				interceptors:      []core.WebSocketInterceptor{global},
				argumentResolvers: wsresolver.Resolvers(nil),
			}
			meta := execution.HandlerMeta{
				Callback: reflect.ValueOf(func(message input.Message[struct{ Text string }]) error {
					log = append(log, "handler")
					if message.Value.Text != "ok" {
						t.Errorf("메시지 값이 잘못됐습니다: %q", message.Value.Text)
					}
					if tt.panicInHandler != nil {
						panic(tt.panicInHandler)
					}
					return tt.handlerErr
				}),
			}
			err := runtime.executeMessage(session{ctx: t.Context()}, 1, []byte(tt.payload), nil, meta, route)
			if !slices.Equal(log, tt.want) {
				t.Fatalf("호출 순서 = %v, 기대 순서 = %v", log, tt.want)
			}
			if tt.wantDecodeError {
				if err == nil || !strings.Contains(err.Error(), "decode WebSocket message") || !errors.Is(global.receivedErr, err) {
					t.Fatalf("실행 오류 = %v, 완료 훅 오류 = %v", err, global.receivedErr)
				}
			} else if !errors.Is(err, tt.wantErr) || !errors.Is(global.receivedErr, tt.wantErr) {
				t.Fatalf("실행 오류 = %v, 완료 훅 오류 = %v, 기대 오류 = %v", err, global.receivedErr, tt.wantErr)
			}
			if tt.handlerErr != nil && !strings.Contains(global.receivedErr.Error(), "no ReturnValueHandler can handle the error return value") {
				t.Fatalf("기존 완료 훅의 핸들러 반환 오류가 바뀌었습니다: %v", global.receivedErr)
			}
			if tt.routeCompletionPanic != nil && (!errors.Is(err, tt.routeCompletionPanic) || !errors.Is(global.receivedErr, tt.routeCompletionPanic)) {
				t.Fatalf("라우트 완료 훅 패닉이 전파되지 않았습니다: %v, %v", err, global.receivedErr)
			}
		})
	}
}
