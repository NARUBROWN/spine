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

func shouldPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("panic이 발생해야 합니다")
		}
	}()
	fn()
}

func TestJoinPath(t *testing.T) {
	if got := joinPath("", "users"); got != "/users" {
		t.Fatalf("leading slash가 보정되어야 합니다: %s", got)
	}
	if got := joinPath("/api", "/users"); got != "/api/users" {
		t.Fatalf("prefix 결합이 잘못되었습니다: %s", got)
	}
}

func TestJoinPath_EmptyPanics(t *testing.T) {
	shouldPanic(t, func() {
		joinPath("", "")
	})
}

func TestAssertNoAmbiguousRoute(t *testing.T) {
	shouldPanic(t, func() {
		assertNoAmbiguousRoute("GET", "/users/:id", []string{"/users/me"})
	})

	assertNoAmbiguousRoute("GET", "/users/:id", []string{"/teams/me"})
	assertNoAmbiguousRoute("GET", "/users/:id/posts", []string{"/users/me"})
}

func TestRun_InvalidGlobalPrefixPanics(t *testing.T) {
	for _, prefix := range []string{"api", "/api/:id", "/api/*"} {
		prefix := prefix
		shouldPanic(t, func() {
			_ = Run(Config{
				HTTP: &boot.HTTPOptions{
					GlobalPrefix: prefix,
				},
			})
		})
	}
}

func TestRun_AmbiguousRoutesPanics(t *testing.T) {
	shouldPanic(t, func() {
		_ = Run(Config{
			HTTP: &boot.HTTPOptions{},
			Routes: []spineRouter.RouteSpec{
				{Method: "GET", Path: "/users/:id", Handler: (*testController).Handle},
				{Method: "GET", Path: "/users/me", Handler: (*testController).Handle},
			},
		})
	})
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
