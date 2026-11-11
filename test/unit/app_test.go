package spine

import (
	"net/http"
	"testing"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/boot"
	contextapi "github.com/NARUBROWN/spine/pkg/spine"
)

type appTestInterceptor struct{}

func (*appTestInterceptor) PreHandle(contextapi.Ctx, core.HandlerMeta) error { return nil }
func (*appTestInterceptor) PostHandle(contextapi.Ctx, core.HandlerMeta)      {}
func (*appTestInterceptor) BeforeResponse(contextapi.Ctx, core.HandlerMeta, error) error {
	return nil
}

func TestAppValidateRejectsNilRouteHandler(t *testing.T) {
	application := New()
	application.Route(http.MethodGet, "/nil", nil)

	err := application.Validate(boot.Options{HTTP: &boot.HTTPOptions{}})
	configErr, ok := err.(*boot.ConfigError)
	if !ok {
		t.Fatalf("Validate error = %T, want *boot.ConfigError", err)
	}
	if len(configErr.Issues) != 1 || configErr.Issues[0].Path != "Routes[0].Handler" {
		t.Fatalf("unexpected validation issues: %+v", configErr.Issues)
	}
}
func (*appTestInterceptor) AfterCompletion(contextapi.Ctx, core.HandlerMeta, error) {
}

func TestAppInterceptorDefaultsToAllTransportsAndSupportsExplicitScope(t *testing.T) {
	application := New().(*app)
	all := &appTestInterceptor{}
	httpOnly := &appTestInterceptor{}
	wsOnly := &appTestInterceptor{}

	application.Interceptor(all)
	application.InterceptorFor(boot.InterceptorHTTP, httpOnly)
	application.InterceptorFor(boot.InterceptorWebSocket, wsOnly)

	want := []boot.InterceptorScope{boot.InterceptorAll, boot.InterceptorHTTP, boot.InterceptorWebSocket}
	if len(application.interceptors) != len(want) {
		t.Fatalf("interceptor bindings = %d, want %d", len(application.interceptors), len(want))
	}
	for i, scope := range want {
		if application.interceptors[i].Scope != scope {
			t.Fatalf("binding[%d] scope = %d, want %d", i, application.interceptors[i].Scope, scope)
		}
	}
}

func TestAppValidateAggregatesWebSocketConfigurationIssues(t *testing.T) {
	application := New()
	err := application.Validate(boot.Options{HTTP: &boot.HTTPOptions{WebSocket: boot.WebSocketOptions{
		MaxConnections:    -2,
		TrustedProxyCIDRs: []string{"not-a-cidr"},
	}}})
	configErr, ok := err.(*boot.ConfigError)
	if !ok {
		t.Fatalf("Validate error = %T, want *boot.ConfigError", err)
	}
	if len(configErr.Issues) != 2 {
		t.Fatalf("issues = %d, want 2: %v", len(configErr.Issues), configErr.Issues)
	}
}

func TestEventCleanupHistoryOptionsRemainIndependent(t *testing.T) {
	application := New(WithEventAdapter("events", nil,
		WithEventPublisherErrorHistory(boot.CleanupErrorHistoryOptions{MaxEntries: 3}),
		WithEventConsumerErrorHistory(boot.CleanupErrorHistoryOptions{MaxEntries: 7}))).(*app)
	options := application.eventAdapters[0].Options
	if options.PublisherErrorHistory.MaxEntries != 3 || options.ConsumerErrorHistory.MaxEntries != 7 {
		t.Fatal(options)
	}
}
