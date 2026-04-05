package bootstrap

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NARUBROWN/spine/core"
	spineRouter "github.com/NARUBROWN/spine/internal/router"
	"github.com/NARUBROWN/spine/pkg/boot"
)

type testController struct{}

func (c *testController) Handle() string { return "ok" }

type testTransport struct {
	initErr    error
	startErr   error
	initCalls  atomic.Int32
	startCalls atomic.Int32
	stopCalls  atomic.Int32
}

func (t *testTransport) Init(container core.Container) error {
	t.initCalls.Add(1)
	return t.initErr
}

func (t *testTransport) Start() error {
	t.startCalls.Add(1)
	return t.startErr
}

func (t *testTransport) Stop(ctx context.Context) error {
	t.stopCalls.Add(1)
	return nil
}

func TestJoinPath(t *testing.T) {
	if got, err := joinPath("", "users"); err != nil || got != "/users" {
		t.Fatalf("leading slash가 보정되어야 합니다: %s", got)
	}
	if got, err := joinPath("/api", "/users"); err != nil || got != "/api/users" {
		t.Fatalf("prefix 결합이 잘못되었습니다: %s", got)
	}
}

func TestJoinPath_EmptyReturnsError(t *testing.T) {
	if _, err := joinPath("", ""); err == nil {
		t.Fatal("빈 path는 에러여야 합니다")
	}
}

func TestAssertNoAmbiguousRoute(t *testing.T) {
	if err := assertNoAmbiguousRoute("GET", "/users/:id", []string{"/users/me"}); err == nil {
		t.Fatal("모호한 라우트는 에러여야 합니다")
	}
	if err := assertNoAmbiguousRoute("GET", "/users/:id", []string{"/teams/me"}); err != nil {
		t.Fatalf("예상하지 못한 에러입니다: %v", err)
	}
	if err := assertNoAmbiguousRoute("GET", "/users/:id/posts", []string{"/users/me"}); err != nil {
		t.Fatalf("예상하지 못한 에러입니다: %v", err)
	}
}

func TestRun_InvalidGlobalPrefixReturnsError(t *testing.T) {
	for _, prefix := range []string{"api", "/api/:id", "/api/*"} {
		prefix := prefix
		err := Run(Config{
			HTTP: &boot.HTTPOptions{
				GlobalPrefix: prefix,
			},
		})
		if err == nil {
			t.Fatalf("잘못된 prefix는 에러여야 합니다: %s", prefix)
		}
	}
}

func TestRun_AmbiguousRoutesReturnsError(t *testing.T) {
	err := Run(Config{
		HTTP: &boot.HTTPOptions{},
		Routes: []spineRouter.RouteSpec{
			{Method: "GET", Path: "/users/:id", Handler: (*testController).Handle},
			{Method: "GET", Path: "/users/me", Handler: (*testController).Handle},
		},
	})
	if err == nil {
		t.Fatal("모호한 라우트는 에러여야 합니다")
	}
}

func TestRun_CustomTransportInitError(t *testing.T) {
	transport := &testTransport{initErr: errors.New("init fail")}

	err := Run(Config{
		CustomTransports: []core.CustomTransport{transport},
		ShutdownTimeout:  10 * time.Millisecond,
	})
	if err == nil || !strings.Contains(err.Error(), "init fail") {
		t.Fatalf("Init 에러가 반환되어야 합니다: %v", err)
	}
	if transport.startCalls.Load() != 0 {
		t.Fatalf("Init 실패 시 Start는 호출되면 안 됩니다: %d", transport.startCalls.Load())
	}
	if transport.stopCalls.Load() != 0 {
		t.Fatalf("Init 실패 시 Stop은 호출되면 안 됩니다: %d", transport.stopCalls.Load())
	}
}

func TestRun_CustomTransportStartError(t *testing.T) {
	transport := &testTransport{startErr: errors.New("start fail")}

	err := Run(Config{
		CustomTransports: []core.CustomTransport{transport},
		ShutdownTimeout:  10 * time.Millisecond,
	})
	if err == nil || !strings.Contains(err.Error(), "start fail") {
		t.Fatalf("Start 에러가 반환되어야 합니다: %v", err)
	}
	if transport.initCalls.Load() != 1 || transport.startCalls.Load() != 1 {
		t.Fatalf("Init/Start 호출 수가 잘못되었습니다: init=%d start=%d", transport.initCalls.Load(), transport.startCalls.Load())
	}
	if transport.stopCalls.Load() != 1 {
		t.Fatalf("Start 실패 후에도 Stop이 호출되어야 합니다: %d", transport.stopCalls.Load())
	}
}

func TestRun_WarmUpResolveFailureReturnsError(t *testing.T) {
	err := Run(Config{
		HTTP: &boot.HTTPOptions{},
		Routes: []spineRouter.RouteSpec{
			{Method: "GET", Path: "/users", Handler: (*testController).Handle},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "Warm-up 실패") {
		t.Fatalf("Warm-up 실패가 에러로 반환되어야 합니다: %v", err)
	}
}

func TestRun_NilGlobalInterceptorReturnsError(t *testing.T) {
	err := Run(Config{
		HTTP:         &boot.HTTPOptions{},
		Interceptors: []core.Interceptor{nil},
	})
	if err == nil || !strings.Contains(err.Error(), "Interceptor가 nil") {
		t.Fatalf("nil 인터셉터는 에러여야 합니다: %v", err)
	}
}
