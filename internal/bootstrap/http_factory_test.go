package bootstrap

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"

	adapter "github.com/NARUBROWN/spine/adapter/http"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/boot"
)

// testHTTPFactory는 커널 lifecycle 테스트만을 위한 서버 fixture입니다.
// 운영용 HTTP 어댑터를 추가하지 않으며 실제 바인딩은 통합 모듈에서 검증합니다.
type testHTTPFactory struct{}

func (testHTTPFactory) Capabilities() adapter.Capabilities {
	return adapter.Capabilities{ContractVersion: adapter.ContractVersion}
}

func (testHTTPFactory) New(adapter.Config) (adapter.HTTPAdapter, error) {
	return &testHTTPAdapter{server: &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})}}, nil
}

type testHTTPAdapter struct{ server *http.Server }

func (a *testHTTPAdapter) Handler() http.Handler              { return a.server.Handler }
func (a *testHTTPAdapter) Serve(listener net.Listener) error  { return a.server.Serve(listener) }
func (a *testHTTPAdapter) Shutdown(ctx context.Context) error { return a.server.Shutdown(ctx) }

type configurableFactory struct {
	capabilities adapter.Capabilities
	newAdapter   func(adapter.Config) (adapter.HTTPAdapter, error)
}

func (f *configurableFactory) Capabilities() adapter.Capabilities { return f.capabilities }
func (f *configurableFactory) New(config adapter.Config) (adapter.HTTPAdapter, error) {
	return f.newAdapter(config)
}

type invalidHandlerAdapter struct{ shutdowns int }

func (*invalidHandlerAdapter) Handler() http.Handler            { return http.HandlerFunc(nil) }
func (*invalidHandlerAdapter) Serve(net.Listener) error         { panic("Serve must not run") }
func (a *invalidHandlerAdapter) Shutdown(context.Context) error { a.shutdowns++; return nil }

func TestHTTPFactoryPreflightRunsBeforeInitialization(t *testing.T) {
	transport := &testTransport{}
	factory := &configurableFactory{
		capabilities: adapter.Capabilities{ContractVersion: "unsupported"},
		newAdapter: func(adapter.Config) (adapter.HTTPAdapter, error) {
			t.Fatal("기능 검사 실패 시 factory.New를 실행하면 안 됩니다")
			return nil, nil
		},
	}
	err := Run(Config{HTTP: &boot.HTTPOptions{}, HTTPAdapterFactory: factory, CustomTransports: []core.CustomTransport{transport}})
	if !errors.Is(err, adapter.ErrInvalidAdapter) || transport.initCalls.Load() != 0 {
		t.Fatalf("어댑터 사전 검증은 전송 초기화보다 먼저여야 합니다: %v", err)
	}
}

func TestHTTPFactoryRejectsInvalidInstancesBeforeServing(t *testing.T) {
	broken := &invalidHandlerAdapter{}
	var typedNil *invalidHandlerAdapter
	for _, instance := range []adapter.HTTPAdapter{nil, typedNil, broken} {
		factory := &configurableFactory{
			capabilities: adapter.Capabilities{ContractVersion: adapter.ContractVersion},
			newAdapter:   func(adapter.Config) (adapter.HTTPAdapter, error) { return instance, nil },
		}
		if err := Run(Config{HTTP: &boot.HTTPOptions{}, HTTPAdapterFactory: factory}); !errors.Is(err, adapter.ErrInvalidAdapter) {
			t.Fatalf("잘못된 인스턴스는 Serve 전에 실패해야 합니다: %v", err)
		}
	}
	if broken.shutdowns != 1 {
		t.Fatalf("준비된 잘못된 handler도 정리해야 합니다: shutdowns=%d", broken.shutdowns)
	}
}
