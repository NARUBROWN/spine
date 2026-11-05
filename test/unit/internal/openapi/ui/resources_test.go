package ui

import (
	"net/url"
	"strings"
	"testing"
)

func TestRelativeURLPreservesStrippedProxyPrefix(t *testing.T) {
	for _, test := range []struct{ page, target string }{
		{"/docs", "/openapi.json"},
		{"/docs", "/docs/_assets/init.js"},
		{"/internal/docs", "/openapi.json"},
		{"/internal/docs", "/internal/spec.json"},
		{"/internal/docs", "/internal/docs/_assets/init.js"},
		{"/docs", "/"},
		{"/internal/docs", "/"},
		{"/internal/docs", "/internal"},
		{"/internal/docs", "/internal/docs"},
		{"/internal/deep/docs", "/internal"},
	} {
		base, _ := url.Parse("https://example.test/service" + test.page)
		relative, _ := url.Parse(RelativeURL(test.page, test.target))
		if got := base.ResolveReference(relative).Path; got != "/service"+test.target {
			t.Fatalf("Proxy prefix was not preserved: %s -> %s = %s", test.page, test.target, got)
		}
	}
}

func TestJSStringCannotCloseScriptElement(t *testing.T) {
	if encoded := JSString("</script><script>alert(1)</script>"); strings.Contains(encoded, "<") {
		t.Fatalf("HTML was not escaped: %s", encoded)
	}
}
