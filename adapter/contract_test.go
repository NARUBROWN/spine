package adapter_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"

	adapter "github.com/NARUBROWN/spine/adapter"
)

// 외부 패키지에서 공개 계약만으로 구현할 수 있는지 컴파일 시 확인합니다.
var (
	_ adapter.Factory     = (*testFactory)(nil)
	_ adapter.HTTPAdapter = (*testHTTPAdapter)(nil)
)

type testFactory struct {
	capabilities      adapter.Capabilities
	capabilitiesCalls int
	newCalls          int
}

func (f *testFactory) Capabilities() adapter.Capabilities {
	f.capabilitiesCalls++
	return f.capabilities
}

func (f *testFactory) New(adapter.Config) (adapter.HTTPAdapter, error) {
	f.newCalls++
	return nil, errors.New("validation must not call Factory.New")
}

// testHTTPAdapter는 실제 서버가 아니라 외부 구현의 메서드 시그니처를 확인하는 fixture입니다.
type testHTTPAdapter struct {
	handler  http.Handler
	serve    func(net.Listener) error
	shutdown func(context.Context) error
}

func (a *testHTTPAdapter) Handler() http.Handler              { return a.handler }
func (a *testHTTPAdapter) Serve(l net.Listener) error         { return a.serve(l) }
func (a *testHTTPAdapter) Shutdown(ctx context.Context) error { return a.shutdown(ctx) }

func TestValidateFactory(t *testing.T) {
	tests := []struct {
		name         string
		capabilities adapter.Capabilities
		required     adapter.Requirements
		wantErr      error
		wantDetails  []string
	}{
		{
			name:         "HTTP_only",
			capabilities: adapter.Capabilities{ContractVersion: adapter.ContractVersion},
		},
		{
			name:         "WebSocket_supported",
			capabilities: adapter.Capabilities{ContractVersion: adapter.ContractVersion, WebSocketUpgrade: true},
			required:     adapter.Requirements{WebSocketUpgrade: true},
		},
		{
			name:         "native_supported",
			capabilities: adapter.Capabilities{ContractVersion: adapter.ContractVersion, NativeHooks: true},
			required:     adapter.Requirements{NativeHooks: true},
		},
		{
			name:         "both_supported",
			capabilities: fullCapabilities(),
			required:     adapter.Requirements{WebSocketUpgrade: true, NativeHooks: true},
		},
		{
			name:         "extra_capabilities_allowed",
			capabilities: fullCapabilities(),
		},
		{
			name:        "missing_version",
			wantErr:     adapter.ErrInvalidAdapter,
			wantDetails: []string{adapter.ContractVersion},
		},
		{
			name:         "different_version",
			capabilities: adapter.Capabilities{ContractVersion: "spine.http-adapter/v2"},
			wantErr:      adapter.ErrInvalidAdapter,
			wantDetails:  []string{"spine.http-adapter/v2", adapter.ContractVersion},
		},
		{
			name:         "version_checked_before_features",
			capabilities: adapter.Capabilities{ContractVersion: "spine.http-adapter/v2"},
			required:     adapter.Requirements{WebSocketUpgrade: true, NativeHooks: true},
			wantErr:      adapter.ErrInvalidAdapter,
		},
		{
			name:         "WebSocket_unsupported",
			capabilities: adapter.Capabilities{ContractVersion: adapter.ContractVersion, NativeHooks: true},
			required:     adapter.Requirements{WebSocketUpgrade: true},
			wantErr:      adapter.ErrUnsupportedCapability,
			wantDetails:  []string{"WebSocketUpgrade"},
		},
		{
			name:         "native_unsupported",
			capabilities: adapter.Capabilities{ContractVersion: adapter.ContractVersion, WebSocketUpgrade: true},
			required:     adapter.Requirements{NativeHooks: true},
			wantErr:      adapter.ErrUnsupportedCapability,
			wantDetails:  []string{"NativeHooks"},
		},
		{
			name:         "one_of_two_requirements_unsupported",
			capabilities: adapter.Capabilities{ContractVersion: adapter.ContractVersion, WebSocketUpgrade: true},
			required:     adapter.Requirements{WebSocketUpgrade: true, NativeHooks: true},
			wantErr:      adapter.ErrUnsupportedCapability,
			wantDetails:  []string{"NativeHooks"},
		},
		{
			name:         "both_unsupported",
			capabilities: adapter.Capabilities{ContractVersion: adapter.ContractVersion},
			required:     adapter.Requirements{WebSocketUpgrade: true, NativeHooks: true},
			wantErr:      adapter.ErrUnsupportedCapability,
			wantDetails:  []string{"WebSocketUpgrade", "NativeHooks"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory := &testFactory{capabilities: tt.capabilities}
			err := adapter.ValidateFactory(factory, tt.required)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("지원되는 구성은 검사를 통과해야 합니다: %v", err)
				}
			} else {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("오류 종류 = %v, 기대값 = %v", err, tt.wantErr)
				}
				wrapped := fmt.Errorf("HTTP setup: %w", err)
				if !errors.Is(wrapped, tt.wantErr) {
					t.Fatalf("상위 오류로 감싸도 원래 종류를 확인할 수 있어야 합니다: %v", wrapped)
				}
				for _, detail := range tt.wantDetails {
					if !strings.Contains(err.Error(), detail) {
						t.Fatalf("오류에 진단 정보 %q가 없습니다: %v", detail, err)
					}
				}
			}
			if factory.capabilitiesCalls != 1 || factory.newCalls != 0 {
				t.Fatalf("기능 선언만 한 번 조회해야 합니다: Capabilities=%d, New=%d", factory.capabilitiesCalls, factory.newCalls)
			}
		})
	}
}

func TestValidateFactoryRejectsNilFactories(t *testing.T) {
	tests := []struct {
		name    string
		factory adapter.Factory
	}{
		{name: "interface"},
		{name: "pointer", factory: (*testFactory)(nil)},
		{name: "function", factory: functionFactory(nil)},
		{name: "map", factory: mapFactory(nil)},
		{name: "slice", factory: sliceFactory(nil)},
		{name: "channel", factory: channelFactory(nil)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := adapter.ValidateFactory(tt.factory, adapter.Requirements{})
			if !errors.Is(err, adapter.ErrInvalidAdapter) || !strings.Contains(err.Error(), "nil") {
				t.Fatalf("타입이 있는 nil도 factory 메서드 호출 전에 거부해야 합니다: %v", err)
			}
		})
	}
}

func TestValidateFactoryAcceptsNonNilFactoryKinds(t *testing.T) {
	tests := []struct {
		name    string
		factory adapter.Factory
	}{
		{name: "value", factory: valueFactory{}},
		{name: "pointer", factory: &testFactory{capabilities: fullCapabilities()}},
		{name: "function", factory: functionFactory(func() {})},
		{name: "map", factory: mapFactory{}},
		{name: "slice", factory: sliceFactory{}},
		{name: "channel", factory: make(channelFactory)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := adapter.ValidateFactory(tt.factory, adapter.Requirements{WebSocketUpgrade: true, NativeHooks: true})
			if err != nil {
				t.Fatalf("nil이 아닌 factory의 종류를 제한해서는 안 됩니다: %v", err)
			}
		})
	}
}

func fullCapabilities() adapter.Capabilities {
	return adapter.Capabilities{
		ContractVersion:  adapter.ContractVersion,
		WebSocketUpgrade: true,
		NativeHooks:      true,
	}
}

// nil을 가질 수 있는 Go 타입도 factory를 구현할 수 있으므로 각각 검사합니다.
type functionFactory func()
type mapFactory map[string]any
type sliceFactory []any
type channelFactory chan struct{}
type valueFactory struct{}

func (functionFactory) Capabilities() adapter.Capabilities { return fullCapabilities() }
func (mapFactory) Capabilities() adapter.Capabilities      { return fullCapabilities() }
func (sliceFactory) Capabilities() adapter.Capabilities    { return fullCapabilities() }
func (channelFactory) Capabilities() adapter.Capabilities  { return fullCapabilities() }
func (valueFactory) Capabilities() adapter.Capabilities    { return fullCapabilities() }

func (functionFactory) New(adapter.Config) (adapter.HTTPAdapter, error) {
	panic("validation must not call Factory.New")
}
func (mapFactory) New(adapter.Config) (adapter.HTTPAdapter, error) {
	panic("validation must not call Factory.New")
}
func (sliceFactory) New(adapter.Config) (adapter.HTTPAdapter, error) {
	panic("validation must not call Factory.New")
}
func (channelFactory) New(adapter.Config) (adapter.HTTPAdapter, error) {
	panic("validation must not call Factory.New")
}
func (valueFactory) New(adapter.Config) (adapter.HTTPAdapter, error) {
	panic("validation must not call Factory.New")
}
