package configuration

import "testing"

func TestOpenAPIPathValidation(t *testing.T) {
	for _, path := range []string{"", "/openapi.json", "/internal/api.json"} {
		if issues := ValidateOpenAPI(OpenAPIOptions{Enabled: true, Path: path}, "HTTP.OpenAPI"); len(issues) != 0 {
			t.Fatalf("정적 경로 %q: %+v", path, issues)
		}
	}
	for _, path := range []string{"relative", "//other", "/a/../b", "/docs/:id", "/docs?x=y", "/docs#x", "/docs/*"} {
		if issues := ValidateOpenAPI(OpenAPIOptions{Enabled: true, Path: path}, "HTTP.OpenAPI"); len(issues) != 1 {
			t.Fatalf("잘못된 경로 %q: %+v", path, issues)
		}
		if issues := ValidateOpenAPI(OpenAPIOptions{Path: path}, "HTTP.OpenAPI"); len(issues) != 0 {
			t.Fatalf("비활성화 상태에서는 사용하지 않는 경로를 검사하면 안 됩니다: %+v", issues)
		}
	}
}
