package bootstrap

import (
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/NARUBROWN/spine/pkg/boot"
)

func TestOpenAPIURLUsesTheBoundPortAndLoopbackForWildcardAddresses(t *testing.T) {
	for _, test := range []struct{ ip, expected string }{
		{"0.0.0.0", "http://127.0.0.1:54321/internal/openapi.json"},
		{"::", "http://[::1]:54321/internal/openapi.json"},
		{"127.0.0.2", "http://127.0.0.2:54321/internal/openapi.json"},
	} {
		if actual := openAPIURL(&net.TCPAddr{IP: net.ParseIP(test.ip), Port: 54321}, "/internal/openapi.json", false); actual != test.expected {
			t.Fatalf("실제 리스너 주소: %s", actual)
		}
	}
}

func TestOpenAPIURLUsesHTTPSForTLS(t *testing.T) {
	if got := openAPIURL(&net.TCPAddr{IP: net.IPv4zero, Port: 8443}, "/openapi.json", true); got != "https://127.0.0.1:8443/openapi.json" {
		t.Fatal(got)
	}
}

func TestRunBuildsOpenAPIUIResourcesOnce(t *testing.T) {
	server := &preparationAdapter{handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})}
	calls := 0
	provider := func(string) ([]boot.OpenAPIResource, error) {
		calls++
		return []boot.OpenAPIResource{{Path: "/docs", ContentType: "text/html", Body: "documentation"}}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := RunContext(ctx, Config{
		Address:            "127.0.0.1:0",
		HTTP:               &boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{Enabled: true, UI: []boot.OpenAPIUI{provider}}},
		HTTPAdapterFactory: preparationFactory(server),
	})
	if err == nil || !strings.Contains(err.Error(), "unexpected Serve") || server.serveCalls.Load() != 1 {
		t.Fatalf("Managed HTTP runtime did not start: %v", err)
	}
	if calls != 1 {
		t.Fatalf("UI provider was called %d times during managed startup; expected 1", calls)
	}
}
