package httpx

import "testing"

func TestCookieValueEncodingRoundTrip(t *testing.T) {
	values := []string{
		"",
		"plain-token",
		"attacker; Domain=example.com\r\nX-Injected: true",
		"한글과 emoji 🍪",
		string([]byte{0x00, 0xff, 0x7f}),
	}

	for _, value := range values {
		encoded := EncodeCookieValue(value)
		decoded, err := DecodeCookieValue(encoded)
		if err != nil {
			t.Fatalf("encoded value must decode: value=%q encoded=%q err=%v", value, encoded, err)
		}
		if decoded != value {
			t.Fatalf("cookie value round trip mismatch: got %q want %q", decoded, value)
		}
		for _, r := range encoded {
			if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '-' && r != '_' {
				t.Fatalf("encoded cookie value contains non-base64url character %q: %q", r, encoded)
			}
		}
	}
}

func TestDecodeCookieValueRejectsMalformedInput(t *testing.T) {
	if decoded, err := DecodeCookieValue("not+base64url"); err == nil {
		t.Fatalf("malformed input must fail, got %q", decoded)
	}
}
