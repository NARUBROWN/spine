package boot

import (
	"testing"
	"time"
)

func TestWebSocketOptionsValidateIssues(t *testing.T) {
	issues := (WebSocketOptions{
		MaxConnections:    -2,
		AllowedOrigins:    []string{"https://example.test/path"},
		TrustedProxyCIDRs: []string{"10.0.0.0/99"},
	}).ValidateIssues("HTTP.WebSocket")
	if len(issues) != 3 {
		t.Fatalf("issues = %d, want 3: %v", len(issues), issues)
	}
	wantCodes := []string{
		"WEBSOCKET_MAX_CONNECTIONS_INVALID",
		"WEBSOCKET_TRUSTED_PROXY_CIDR_INVALID",
		"WEBSOCKET_ALLOWED_ORIGIN_INVALID",
	}
	for i, code := range wantCodes {
		if issues[i].Code != code {
			t.Fatalf("issue[%d].Code = %q, want %q", i, issues[i].Code, code)
		}
	}
}

func TestWebSocketOptionsNamedConnectionLimits(t *testing.T) {
	if DefaultWebSocketMaxConnections <= 0 {
		t.Fatal("default WebSocket connection limit must be finite and positive")
	}
	if UnlimitedWebSocketConnections >= 0 {
		t.Fatal("unlimited WebSocket connection expression must be negative")
	}
	if err := (WebSocketOptions{MaxConnections: UnlimitedWebSocketConnections}).Validate(); err != nil {
		t.Fatalf("explicit unlimited connection setting must be valid: %v", err)
	}
}

func TestWebSocketOptionsRejectsNegativeBoundsAndTimeouts(t *testing.T) {
	issues := (WebSocketOptions{
		MaxMessageBytes:  -1,
		HandshakeTimeout: -time.Second,
		ReadTimeout:      -time.Second,
		WriteTimeout:     -time.Second,
		PingInterval:     -time.Second,
	}).ValidateIssues("HTTP.WebSocket")
	want := []string{
		"WEBSOCKET_MAX_MESSAGE_BYTES_INVALID",
		"WEBSOCKET_HANDSHAKE_TIMEOUT_INVALID",
		"WEBSOCKET_READ_TIMEOUT_INVALID",
		"WEBSOCKET_WRITE_TIMEOUT_INVALID",
		"WEBSOCKET_PING_INTERVAL_INVALID",
	}
	if len(issues) != len(want) {
		t.Fatalf("issues = %d, want %d: %v", len(issues), len(want), issues)
	}
	for i, code := range want {
		if issues[i].Code != code {
			t.Fatalf("issue[%d].Code = %q, want %q", i, issues[i].Code, code)
		}
	}
}

func TestHTTPOptionsRejectsNegativeTimeoutsAndHeaderLimitButAllowsUnlimitedBody(t *testing.T) {
	issues := (HTTPOptions{
		ReadHeaderTimeout: -time.Second,
		ReadTimeout:       -time.Second,
		WriteTimeout:      -time.Second,
		IdleTimeout:       -time.Second,
		MaxHeaderBytes:    -1,
		MaxBodyBytes:      -1,
	}).ValidateIssues("HTTP")
	want := []string{
		"HTTP_READ_HEADER_TIMEOUT_INVALID",
		"HTTP_READ_TIMEOUT_INVALID",
		"HTTP_WRITE_TIMEOUT_INVALID",
		"HTTP_IDLE_TIMEOUT_INVALID",
		"HTTP_MAX_HEADER_BYTES_INVALID",
	}
	if len(issues) != len(want) {
		t.Fatalf("issues = %d, want %d: %v", len(issues), len(want), issues)
	}
	for i, code := range want {
		if issues[i].Code != code {
			t.Fatalf("issue[%d].Code = %q, want %q", i, issues[i].Code, code)
		}
	}
}
