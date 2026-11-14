package spine

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/constructor"
	"github.com/NARUBROWN/spine/pkg/httpx"
)

type workflowService struct{ marker byte }

type workflowTransport struct {
	instance *workflowService
	started  chan struct{}
	stopped  atomic.Bool
}

func (t *workflowTransport) Init(registry core.Container) error {
	dependency, err := registry.Resolve(reflect.TypeFor[*workflowService]())
	if err != nil {
		return err
	}
	t.instance = dependency.(*workflowService)
	return nil
}
func (t *workflowTransport) Start() error {
	close(t.started)
	return nil
}
func (t *workflowTransport) Stop(context.Context) error { t.stopped.Store(true); return nil }

func TestShutdownHookRunsAfterTransportStopWithSharedDependency(t *testing.T) {
	transport := &workflowTransport{started: make(chan struct{})}
	cause := errors.New("worker persistence failed")
	var hookCalls atomic.Int32
	application := New(WithShutdownHook(func(ctx context.Context, service *workflowService) error {
		hookCalls.Add(1)
		if _, ok := ctx.Deadline(); !ok || ctx.Err() != nil || service != transport.instance || !transport.stopped.Load() {
			return errors.New("invalid shutdown context, dependency or order")
		}
		return cause
	}))
	application.Constructor(constructor.Custom(func() (*workflowService, error) { return &workflowService{}, nil }))
	application.RegisterTransport(transport)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- application.RunContext(ctx, boot.Options{ShutdownTimeout: time.Second}) }()
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-transport.started:
	}
	cancel()
	if err := <-done; !errors.Is(err, cause) || hookCalls.Load() != 1 {
		t.Fatalf("calls=%d err=%v", hookCalls.Load(), err)
	}
}

func TestPreparedShutdownHooksRunOnceAndContinueAfterError(t *testing.T) {
	cause := errors.New("cleanup failed")
	var calls []int
	application := New(
		WithShutdownHook(func(context.Context) error { calls = append(calls, 1); return cause }),
		WithShutdownHook(func(context.Context) error { calls = append(calls, 2); return nil }),
	)
	prepared, err := application.Prepare(context.Background(), boot.Options{})
	if err != nil || len(calls) != 0 {
		t.Fatalf("calls=%v err=%v", calls, err)
	}
	for i := 0; i < 2; i++ {
		if err := prepared.Close(context.Background()); !errors.Is(err, cause) {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(calls, []int{1, 2}) {
		t.Fatal(calls)
	}
}

func TestShutdownHookPreparationChecksCallbacksAndDependencies(t *testing.T) {
	var nilHook func(context.Context) error
	if err := New(WithShutdownHook(nilHook)).Validate(boot.Options{}); err == nil {
		t.Fatal("accepted nil shutdown hook")
	}
	application := New(WithShutdownHook(func(context.Context, *workflowService) error {
		t.Fatal("hook ran during preparation")
		return nil
	}))
	if _, err := application.Prepare(context.Background(), boot.Options{}); err == nil {
		t.Fatal("accepted missing dependency")
	}
}

type unusedWorkflowDependency struct{}

func (*unusedWorkflowDependency) Constructor(*workflowService) *unusedWorkflowDependency {
	panic("service ran after repository failure")
}

func TestFallibleRepositoryPreventsHTTPControllerAndServiceCreation(t *testing.T) {
	cause := errors.New("could not restore repository")
	for _, run := range []func(App, boot.Options) error{
		func(app App, options boot.Options) error {
			_, err := app.Prepare(t.Context(), options)
			return err
		},
		func(app App, options boot.Options) error { return app.RunContext(t.Context(), options) },
	} {
		app := New(WithHTTPAdapter(multipleResponseFactory{}))
		app.Constructor(
			constructor.Custom(func() (*workflowService, error) { return nil, cause }),
			constructor.Service(&unusedWorkflowDependency{}),
			func(*unusedWorkflowDependency) *multipleResponseController {
				t.Error("controller ran after repository failure")
				return &multipleResponseController{}
			},
		)
		options := boot.Options{HTTP: &boot.HTTPOptions{}}
		if err := app.Validate(options); err != nil {
			t.Fatal(err)
		}
		if err := run(app, options); !errors.Is(err, cause) {
			t.Fatal(err)
		}
	}
}

func TestFallibleControllerIsCollectedDuringPrepare(t *testing.T) {
	var built bool
	app := New(WithHTTPAdapter(multipleResponseFactory{}))
	app.Constructor(func() (*multipleResponseController, error) {
		built = true
		return &multipleResponseController{handler: func() (httpx.Response[string], httpx.Response[int]) {
			return httpx.Response[string]{}, httpx.Response[int]{}
		}}, nil
	})
	_, err := app.Prepare(t.Context(), boot.Options{HTTP: &boot.HTTPOptions{}})
	var configError *boot.ConfigError
	if !built || !errors.As(err, &configError) || len(configError.Issues) != 1 ||
		configError.Issues[0].Code != "HTTP_RESPONSE_RETURNS_DUPLICATED" {
		t.Fatalf("built=%v err=%v", built, err)
	}
}
