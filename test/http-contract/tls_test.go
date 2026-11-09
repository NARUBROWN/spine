package contractprobe

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	contract "github.com/NARUBROWN/spine/contract/http"
)

func TestTLSContractAcrossNativeEngines(t *testing.T) {
	seed := httptest.NewTLSServer(http.NotFoundHandler())
	defer seed.Close()
	for _, engine := range engines {
		for _, explicit := range []bool{false, true} {
			t.Run(engine.name+"/"+map[bool]string{false: "prepared", true: "explicit"}[explicit], func(t *testing.T) {
				factory := engine.factory(hooks{})
				config := contract.Config{Execute: func(scope contract.HTTPContext) error {
					data, err := io.ReadAll(scope.Body())
					if err != nil {
						return err
					}
					return scope.Writer().WriteBytes(200, data)
				}}
				tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, Certificates: seed.TLS.Certificates}
				var options []contract.ServeOption
				if explicit {
					options = []contract.ServeOption{contract.WithTLS(tlsConfig)}
				} else {
					config.Serve = contract.ServeOptions{TLS: tlsConfig}
				}
				if err := contract.ValidateFactory(factory, contract.Requirements{TLS: true}); err != nil {
					t.Fatal(err)
				}
				server, err := factory.New(config)
				if err != nil {
					t.Fatal(err)
				}
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				done := make(chan error, 1)
				go func() { done <- server.Serve(listener, options...) }()
				transport := seed.Client().Transport.(*http.Transport).Clone()
				client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
				t.Cleanup(func() {
					transport.CloseIdleConnections()
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
					defer cancel()
					if err := server.Shutdown(ctx); err != nil {
						t.Error(err)
					}
					if err := receive(t, done); !errors.Is(err, contract.ErrServerClosed) {
						t.Errorf("TLS shutdown: %v", err)
					}
				})
				response, err := client.Get("https://" + listener.Addr().String() + "/tls")
				if err != nil {
					t.Fatal(err)
				}
				data, err := io.ReadAll(response.Body)
				_ = response.Body.Close()
				if err != nil || response.StatusCode != 200 || len(data) != 0 || response.TLS == nil || len(response.TLS.VerifiedChains) == 0 {
					t.Fatalf("TLS native request: status=%d read=%v", response.StatusCode, err)
				}
			})
		}
	}
}
