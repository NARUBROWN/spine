package boot

import "github.com/NARUBROWN/spine/internal/openapi/resourcepath"

// OpenAPIOptions는 명시적으로 활성화하는 HTTP 명세 리소스를 설정합니다.
// Enabled가 true일 때만 자동 생성하고 제공합니다. 기본값은 비활성화입니다.
// 활성화 시 기본 경로는 /openapi.json이며 API GlobalPrefix와 독립적입니다.
// 명세는 준비 단위로 메모리에 보관하고 파일을 자동으로 쓰지 않습니다.
type OpenAPIOptions struct {
	Enabled bool
	Path    string
	Title   string
	Version string
	// UI는 선택 패키지가 제공하는 문서 화면과 정적 파일입니다.
	// Enabled가 true일 때만 연결하며 생략하면 JSON 명세만 제공합니다.
	UI []OpenAPIUI
}

// OpenAPIResource는 GET과 HEAD로 제공하는 불변 문서 리소스입니다.
// Path는 GlobalPrefix와 독립적인 정적 절대 경로입니다.
type OpenAPIResource struct {
	Path        string
	ContentType string
	Body        string
}

// OpenAPIUI는 명세 경로에 연결할 정적 리소스를 생성합니다.
// Validate, Prepare 또는 Run의 사전 검사에서 호출합니다.
// 제공자는 외부 자원을 열거나 사용자 핸들러를 실행하면 안 됩니다.
// Prepare와 Run은 사전 검사에서 반환한 리소스를 그대로 제공합니다.
type OpenAPIUI func(documentPath string) ([]OpenAPIResource, error)

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
	if !resourcepath.Valid(p) {
		return []ConfigIssue{{Path: prefix + ".Path", Code: "OPENAPI_PATH_INVALID", Message: "OpenAPI path must be a static absolute HTTP path.", Hint: "Use /openapi.json or another static path."}}
	}
	return nil
}
