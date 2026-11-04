package boot

import (
	"path"
	"strings"
	"unicode"
)

// OpenAPIOptions는 명시적으로 활성화하는 HTTP 명세 리소스를 설정합니다.
// Enabled가 true일 때만 자동 생성하고 제공합니다. 기본값은 비활성화입니다.
// 활성화 시 기본 경로는 /openapi.json이며 API GlobalPrefix와 독립적입니다.
// 명세는 준비 단위로 메모리에 보관하고 파일을 자동으로 쓰지 않습니다.
type OpenAPIOptions struct {
	Enabled bool
	Path    string
	Title   string
	Version string
}

func (o OpenAPIOptions) DocumentPath() string {
	if o.Path == "" {
		return "/openapi.json"
	}
	return o.Path
}

func (o OpenAPIOptions) ValidateIssues(prefix string) []ConfigIssue {
	if !o.Enabled {
		return nil
	}
	p := o.DocumentPath()
	if !strings.HasPrefix(p, "/") || strings.ContainsAny(p, ":*?#%\\\r\n\t ") || path.Clean(p) != p || strings.IndexFunc(p, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return []ConfigIssue{{Path: prefix + ".Path", Code: "OPENAPI_PATH_INVALID", Message: "OpenAPI path must be a static absolute HTTP path.", Hint: "Use /openapi.json or another static path."}}
	}
	return nil
}
