package bootstrap

import (
	"net"
	"testing"
)

func TestOpenAPIURLUsesTheBoundPortAndLoopbackForWildcardAddresses(t *testing.T) {
	for _, test := range []struct{ ip, expected string }{
		{"0.0.0.0", "http://127.0.0.1:54321/internal/openapi.json"},
		{"::", "http://[::1]:54321/internal/openapi.json"},
		{"127.0.0.2", "http://127.0.0.2:54321/internal/openapi.json"},
	} {
		if actual := openAPIURL(&net.TCPAddr{IP: net.ParseIP(test.ip), Port: 54321}, "/internal/openapi.json"); actual != test.expected {
			t.Fatalf("실제 리스너 주소: %s", actual)
		}
	}
}
