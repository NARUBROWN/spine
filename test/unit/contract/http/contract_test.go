package httpcontract_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	httpcontract "github.com/NARUBROWN/spine/contract/http"
)

type factory struct {
	caps httpcontract.Capabilities
}

func (f *factory) Capabilities() httpcontract.Capabilities { return f.caps }
func (*factory) New(httpcontract.Config) (httpcontract.HTTPAdapter, error) {
	panic("validation must not construct an adapter")
}

type upgradeFactory struct{ factory }

func (*upgradeFactory) NewWithNetHTTPUpgrades(httpcontract.Config, []httpcontract.NetHTTPUpgradeRoute) (httpcontract.HTTPAdapter, error) {
	panic("validation must not construct an upgrade adapter")
}

// neutralAdapter는 net/http handler 없이 필수 계약을 구현합니다.
type neutralAdapter struct{}

func (*neutralAdapter) Serve(net.Listener) error { panic("validation must not start a server") }
func (*neutralAdapter) StopAccepting()           { panic("validation must not stop requests") }
func (*neutralAdapter) Shutdown(context.Context) error {
	panic("validation must not shut down a server")
}

type nativeAdapter struct {
	neutralAdapter
	handler http.Handler
}

func (a *nativeAdapter) Handler() http.Handler { return a.handler }

type nilHandler struct{}

func (*nilHandler) ServeHTTP(http.ResponseWriter, *http.Request) {
	panic("validation must not serve a request")
}

func capabilities() httpcontract.Capabilities {
	return httpcontract.Capabilities{ContractVersion: httpcontract.ContractVersion}
}

func config() httpcontract.Config {
	return httpcontract.Config{Execute: func(httpcontract.HTTPContext) error {
		panic("validation must not execute a request")
	}}
}

func TestFactoryRejectsInvalidAssemblyBeforeConstruction(t *testing.T) {
	var nilFactory *factory
	cases := []struct {
		name    string
		factory httpcontract.Factory
		want    error
	}{
		{"nil", nil, httpcontract.ErrInvalidAdapter},
		{"typed nil", nilFactory, httpcontract.ErrInvalidAdapter},
		{"v2", &factory{caps: httpcontract.Capabilities{ContractVersion: "spine.http-adapter/v2"}}, httpcontract.ErrInvalidAdapter},
		{"unknown version", &factory{caps: httpcontract.Capabilities{ContractVersion: "spine.http-adapter/v99"}}, httpcontract.ErrInvalidAdapter},
		{"upgrade declaration without extension", &factory{caps: httpcontract.Capabilities{ContractVersion: httpcontract.ContractVersion, NetHTTPUpgrade: true}}, httpcontract.ErrInvalidAdapter},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := httpcontract.ValidateFactory(tc.factory, httpcontract.Requirements{}); !errors.Is(err, tc.want) {
				t.Fatalf("ValidateFactory = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestFactoryRequiresOnlySelectedOptionalFeatures(t *testing.T) {
	f := &factory{caps: capabilities()}
	if err := httpcontract.ValidateFactory(f, httpcontract.Requirements{}); err != nil {
		t.Fatalf("engine without native net/http features: %v", err)
	}
	for _, required := range []httpcontract.Requirements{
		{NetHTTPHandler: true},
		{NetHTTPUpgrade: true},
		{ClientDisconnectCancellation: true},
	} {
		if err := httpcontract.ValidateFactory(f, required); !errors.Is(err, httpcontract.ErrUnsupportedCapability) {
			t.Fatalf("requirements %+v = %v", required, err)
		}
	}
	caps := capabilities()
	caps.NetHTTPHandler, caps.NetHTTPUpgrade, caps.ClientDisconnectCancellation = true, true, true
	u := &upgradeFactory{factory{caps: caps}}
	if err := httpcontract.ValidateFactory(u, httpcontract.Requirements{NetHTTPHandler: true, NetHTTPUpgrade: true, ClientDisconnectCancellation: true}); err != nil {
		t.Fatal(err)
	}
}

func TestAdapterValidationKeepsNativeHandlerOptional(t *testing.T) {
	if err := httpcontract.ValidateAdapter(&neutralAdapter{}, capabilities()); err != nil {
		t.Fatal(err)
	}
	if _, err := httpcontract.NetHTTPHandler(&neutralAdapter{}); !errors.Is(err, httpcontract.ErrUnsupportedCapability) {
		t.Fatalf("native helper = %v", err)
	}
	var nilAdapter *neutralAdapter
	for _, adapter := range []httpcontract.HTTPAdapter{nil, nilAdapter} {
		if err := httpcontract.ValidateAdapter(adapter, capabilities()); !errors.Is(err, httpcontract.ErrInvalidAdapter) {
			t.Fatalf("nil adapter = %v", err)
		}
		if _, err := httpcontract.NetHTTPHandler(adapter); !errors.Is(err, httpcontract.ErrInvalidAdapter) {
			t.Fatalf("nil native helper = %v", err)
		}
	}
	caps := capabilities()
	caps.NetHTTPHandler = true
	if err := httpcontract.ValidateAdapter(&neutralAdapter{}, caps); !errors.Is(err, httpcontract.ErrInvalidAdapter) {
		t.Fatalf("declared handler without provider = %v", err)
	}
}

func TestNativeHandlerValidationRejectsTypedNil(t *testing.T) {
	var nilPointer *nilHandler
	var nilFunction http.HandlerFunc
	caps := capabilities()
	caps.NetHTTPHandler = true
	for _, handler := range []http.Handler{nil, nilPointer, nilFunction} {
		adapter := &nativeAdapter{handler: handler}
		if _, err := httpcontract.NetHTTPHandler(adapter); !errors.Is(err, httpcontract.ErrInvalidAdapter) {
			t.Fatalf("handler %T: %v", handler, err)
		}
		if err := httpcontract.ValidateAdapter(adapter, caps); !errors.Is(err, httpcontract.ErrInvalidAdapter) {
			t.Fatalf("adapter with handler %T: %v", handler, err)
		}
	}
	want := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("must not run") })
	adapter := &nativeAdapter{handler: want}
	if err := httpcontract.ValidateAdapter(adapter, caps); err != nil {
		t.Fatal(err)
	}
	handler, err := httpcontract.NetHTTPHandler(adapter)
	if err != nil || handler == nil {
		t.Fatalf("handler = %v, error = %v", handler, err)
	}
}

func TestUpgradeRouteValidationDoesNotInvokeHandlers(t *testing.T) {
	var nilPointer *nilHandler
	var nilFunction http.HandlerFunc
	for _, route := range []httpcontract.NetHTTPUpgradeRoute{
		{},
		{Path: "/ws"},
		{Handler: &nilHandler{}},
		{Path: "/ws", Handler: nilPointer},
		{Path: "/ws", Handler: nilFunction},
	} {
		if err := httpcontract.ValidateNetHTTPUpgradeRoutes([]httpcontract.NetHTTPUpgradeRoute{route}); !errors.Is(err, httpcontract.ErrInvalidAdapter) {
			t.Fatalf("route %+v = %v", route, err)
		}
	}
	if err := httpcontract.ValidateNetHTTPUpgradeRoutes([]httpcontract.NetHTTPUpgradeRoute{{Path: "/ws", Handler: &nilHandler{}}}); err != nil {
		t.Fatal(err)
	}
}

func TestConfigRejectsUnsupportedExplicitOptions(t *testing.T) {
	cases := []struct {
		name    string
		options httpcontract.ServerOptions
		support httpcontract.ServerOptionSupport
	}{
		{"DisableRecover", httpcontract.ServerOptions{DisableRecover: true}, httpcontract.ServerOptionSupport{DisableRecover: true}},
		{"ReadHeaderTimeout", httpcontract.ServerOptions{ReadHeaderTimeout: time.Second}, httpcontract.ServerOptionSupport{ReadHeaderTimeout: true}},
		{"ReadTimeout", httpcontract.ServerOptions{ReadTimeout: time.Second}, httpcontract.ServerOptionSupport{ReadTimeout: true}},
		{"WriteTimeout", httpcontract.ServerOptions{WriteTimeout: time.Second}, httpcontract.ServerOptionSupport{WriteTimeout: true}},
		{"IdleTimeout", httpcontract.ServerOptions{IdleTimeout: time.Second}, httpcontract.ServerOptionSupport{IdleTimeout: true}},
		{"MaxHeaderBytes", httpcontract.ServerOptions{MaxHeaderBytes: 1024}, httpcontract.ServerOptionSupport{MaxHeaderBytes: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config()
			cfg.Options = tc.options
			if err := cfg.Validate(capabilities()); !errors.Is(err, httpcontract.ErrUnsupportedOption) || !strings.Contains(err.Error(), tc.name) {
				t.Fatalf("unsupported option = %v", err)
			}
			caps := capabilities()
			caps.ServerOptions = tc.support
			if err := cfg.Validate(caps); err != nil {
				t.Fatalf("supported option = %v", err)
			}
		})
	}
}

func TestConfigRejectsMalformedValuesBeforeSupportChecks(t *testing.T) {
	for _, options := range []httpcontract.ServerOptions{
		{ReadHeaderTimeout: -1}, {ReadTimeout: -1}, {WriteTimeout: -1}, {IdleTimeout: -1}, {MaxHeaderBytes: -1},
	} {
		cfg := config()
		cfg.Options = options
		if err := cfg.Validate(capabilities()); !errors.Is(err, httpcontract.ErrInvalidAdapter) || errors.Is(err, httpcontract.ErrUnsupportedOption) {
			t.Fatalf("invalid option %+v = %v", options, err)
		}
	}
	if err := (httpcontract.Config{}).Validate(capabilities()); !errors.Is(err, httpcontract.ErrInvalidAdapter) {
		t.Fatalf("nil Execute = %v", err)
	}
	caps := capabilities()
	caps.ContractVersion = "spine.http-adapter/v2"
	if err := config().Validate(caps); !errors.Is(err, httpcontract.ErrInvalidAdapter) {
		t.Fatalf("v2 configuration = %v", err)
	}
}

func TestBodyLimitPolicyDoesNotResolveDefaultsOrRequireNativeOptions(t *testing.T) {
	for _, limit := range []int64{-100, -1, 0, 1, 32 << 20} {
		cfg := config()
		cfg.Options.MaxBodyBytes = limit
		if err := cfg.Validate(capabilities()); err != nil {
			t.Fatalf("limit %d: %v", limit, err)
		}
		if cfg.Options.MaxBodyBytes != limit {
			t.Fatalf("validation rewrote limit %d", limit)
		}
	}
}

func TestRequestErrorKeepsClassificationAndOriginalCause(t *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded, errors.New("reader failed")} {
		original := &httpcontract.RequestError{Status: http.StatusRequestEntityTooLarge, Cause: cause}
		wrapped := fmt.Errorf("body input: %w", original)
		var input *httpcontract.RequestError
		if !errors.As(wrapped, &input) || input.Status != http.StatusRequestEntityTooLarge || !errors.Is(wrapped, cause) {
			t.Fatalf("cause not preserved: %v", wrapped)
		}
		if input.Error() != http.StatusText(http.StatusRequestEntityTooLarge) {
			t.Fatalf("default message = %q", input.Error())
		}
	}
	if text := (&httpcontract.RequestError{Status: 400, Message: "invalid body"}).Error(); text != "invalid body" {
		t.Fatalf("message = %q", text)
	}
	wrapped := fmt.Errorf("serve result: %w", httpcontract.ErrServerClosed)
	if !errors.Is(wrapped, httpcontract.ErrServerClosed) || errors.Is(wrapped, http.ErrServerClosed) {
		t.Fatalf("normal exit is not an engine-neutral signal: %v", wrapped)
	}
}
