package boot

import "testing"

func TestOpenAPIPathValidation(t *testing.T) {
	for _, path := range []string{"", "/openapi.json", "/internal/api.json"} {
		if issues := (OpenAPIOptions{Path: path}).ValidateIssues("HTTP.OpenAPI"); len(issues) != 0 {
			t.Fatalf("정적 경로 %q: %+v", path, issues)
		}
	}
	for _, path := range []string{"relative", "//other", "/a/../b", "/docs/:id", "/docs?x=y", "/docs#x", "/docs/*"} {
		if issues := (OpenAPIOptions{Path: path}).ValidateIssues("HTTP.OpenAPI"); len(issues) != 1 {
			t.Fatalf("잘못된 경로 %q: %+v", path, issues)
		}
	}
}
