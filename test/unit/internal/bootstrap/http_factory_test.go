package bootstrap

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	adapter "github.com/NARUBROWN/spine/contract/http"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/boot"
)

// testHTTPFactory는 커널 lifecycle 테스트만을 위한 서버 fixture입니다.
// 운영용 HTTP 어댑터를 추가하지 않으며 실제 바인딩은 통합 모듈에서 검증합니다.
type testHTTPFactory struct{}

func (testHTTPFactory) Capabilities() adapter.Capabilities {
	return adapter.Capabilities{ContractVersion: adapter.ContractVersion, NetHTTPHandler: true}
}

func (testHTTPFactory) New(adapter.Config) (adapter.HTTPAdapter, error) {
	return &testHTTPAdapter{server: &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})}}, nil
}

type testHTTPAdapter struct{ server *http.Server }

func (a *testHTTPAdapter) StopAccepting()        {}
func (a *testHTTPAdapter) Handler() http.Handler { return a.server.Handler }
func (a *testHTTPAdapter) Serve(listener net.Listener, _ ...adapter.ServeOption) error {
	err := a.server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return adapter.ErrServerClosed
	}
	return err
}
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

func (*invalidHandlerAdapter) StopAccepting()        {}
func (*invalidHandlerAdapter) Handler() http.Handler { return http.HandlerFunc(nil) }
func (*invalidHandlerAdapter) Serve(net.Listener, ...adapter.ServeOption) error {
	panic("Serve must not run")
}
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

func TestUnsupportedHTTPOptionFailsBeforeInitialization(t *testing.T) {
	transport := &testTransport{}
	factory := &configurableFactory{
		capabilities: adapter.Capabilities{ContractVersion: adapter.ContractVersion},
		newAdapter: func(adapter.Config) (adapter.HTTPAdapter, error) {
			t.Fatal("미지원 옵션이면 어댑터를 만들면 안 됩니다")
			return nil, nil
		},
	}
	err := Validate(Config{HTTP: &boot.HTTPOptions{WriteTimeout: time.Second}, HTTPAdapterFactory: factory, CustomTransports: []core.CustomTransport{transport}})
	if !errors.Is(err, adapter.ErrUnsupportedOption) || transport.initCalls.Load() != 0 {
		t.Fatalf("명시적인 미지원 native 옵션을 준비 전에 거부해야 합니다: %v", err)
	}
}

func TestTLSPreflightRunsBeforeFactoriesAndTransports(t *testing.T) {
	for _, supported := range []bool{false, true} {
		t.Run(map[bool]string{false: "unsupported", true: "invalid"}[supported], func(t *testing.T) {
			transport := &testTransport{}
			factoryCalls, providerCalls := 0, 0
			config := &tls.Config{GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) { providerCalls++; return nil, nil }}
			if supported {
				config = &tls.Config{}
			}
			factory := &configurableFactory{capabilities: adapter.Capabilities{ContractVersion: adapter.ContractVersion, TLS: supported}, newAdapter: func(adapter.Config) (adapter.HTTPAdapter, error) { factoryCalls++; return &engineNeutralAdapter{}, nil }}
			_, err := Prepare(context.Background(), Config{HTTP: &boot.HTTPOptions{TLS: config}, HTTPAdapterFactory: factory, CustomTransports: []core.CustomTransport{transport}})
			if err == nil || factoryCalls != 0 || providerCalls != 0 || transport.initCalls.Load() != 0 {
				t.Fatalf("TLS 사전 검사: %v factory=%d provider=%d init=%d", err, factoryCalls, providerCalls, transport.initCalls.Load())
			}
			if !supported && !errors.Is(err, adapter.ErrUnsupportedCapability) {
				t.Fatal(err)
			}
		})
	}
}

func TestPreparedTLSConfigurationIsAnIndependentSnapshot(t *testing.T) {
	calls := 0
	original := &tls.Config{MinVersion: tls.VersionTLS12, NextProtos: []string{"http/1.1"}, GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) { calls++; return nil, nil }}
	var received *tls.Config
	factory := &configurableFactory{capabilities: adapter.Capabilities{ContractVersion: adapter.ContractVersion, TLS: true}, newAdapter: func(config adapter.Config) (adapter.HTTPAdapter, error) {
		received = config.Serve.TLS
		return &engineNeutralAdapter{}, nil
	}}
	p, err := Prepare(context.Background(), Config{HTTP: &boot.HTTPOptions{TLS: original}, HTTPAdapterFactory: factory})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close(context.Background())
	original.NextProtos[0] = "changed"
	original.MinVersion = tls.VersionTLS13
	if received == original || received.NextProtos[0] != "http/1.1" || received.MinVersion != tls.VersionTLS12 || calls != 0 {
		t.Fatal("준비한 TLS 설정의 격리와 콜백 무실행을 보장해야 합니다")
	}
}

type engineNeutralAdapter struct{ stopped bool }

func (*engineNeutralAdapter) Serve(net.Listener, ...adapter.ServeOption) error {
	panic("Prepare must not Serve")
}
func (a *engineNeutralAdapter) StopAccepting()               { a.stopped = true }
func (*engineNeutralAdapter) Shutdown(context.Context) error { return nil }

func TestPrepareDoesNotRequireNetHTTPHandler(t *testing.T) {
	server := &engineNeutralAdapter{}
	factory := &configurableFactory{
		capabilities: adapter.Capabilities{ContractVersion: adapter.ContractVersion},
		newAdapter: func(c adapter.Config) (adapter.HTTPAdapter, error) {
			if c.Options.MaxBodyBytes != 32<<20 {
				t.Fatalf("기존 기본 본문 한도: %d", c.Options.MaxBodyBytes)
			}
			return server, nil
		},
	}
	prepared, err := Prepare(context.Background(), Config{HTTP: &boot.HTTPOptions{}, HTTPAdapterFactory: factory})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.HTTPAdapter() != server {
		t.Fatal("준비한 동일 어댑터를 반환해야 합니다")
	}
	if _, err := adapter.NetHTTPHandler(prepared.HTTPAdapter()); !errors.Is(err, adapter.ErrUnsupportedCapability) {
		t.Fatal(err)
	}
	if err := prepared.Close(context.Background()); err != nil || !server.stopped {
		t.Fatalf("native handler 없이 차단과 종료를 처리해야 합니다: %v", err)
	}
}

func TestHTTPFactoryRejectsInvalidInstancesBeforeServing(t *testing.T) {
	broken := &invalidHandlerAdapter{}
	var typedNil *invalidHandlerAdapter
	for _, instance := range []adapter.HTTPAdapter{nil, typedNil, broken} {
		factory := &configurableFactory{
			capabilities: adapter.Capabilities{ContractVersion: adapter.ContractVersion, NetHTTPHandler: true},
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
