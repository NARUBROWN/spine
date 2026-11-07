// Package redoc는 ReDoc과 재배포 고지를 단일 바이너리에 포함합니다.
package redoc

import (
	_ "embed"
	"html"
	"strings"

	"github.com/NARUBROWN/spine/internal/openapi/ui"
	"github.com/NARUBROWN/spine/pkg/boot"
)

//go:embed assets/redoc.standalone.js
var javascript string

// 상위 UI의 외부 로고 URL을 비워 기존 텍스트 대체 표시를 사용합니다.
// 원본 번들과 저작권 고지는 assets/에 그대로 보관합니다.
var offlineJavascript = strings.ReplaceAll(javascript, "https://cdn.redoc.ly/redoc/logo-mini.svg", "data:,")

//go:embed assets/LICENSE
var license string

//go:embed assets/THIRD_PARTY_LICENSES.txt
var thirdPartyLicenses string

//go:embed assets/redoc.standalone.js.LICENSE.txt
var bundleLicenses string

// Options는 ReDoc의 경로와 브라우저 제목을 설정합니다.
type Options struct {
	// Path를 생략하면 /redoc를 사용합니다. GlobalPrefix와 독립적입니다.
	Path string
	// Title을 생략하면 API reference를 사용합니다.
	Title string
}

// New는 네트워크 접근 없이 임베드한 화면과 정적 파일을 연결합니다.
// 반환값을 boot.OpenAPIOptions.UI에 넣고 Enabled를 true로 설정하세요.
func New(options Options) boot.OpenAPIUI {
	return func(documentPath string) ([]boot.OpenAPIResource, error) {
		page := options.Path
		if page == "" {
			page = "/redoc"
		}
		if err := ui.ValidatePaths(page, documentPath); err != nil {
			return nil, err
		}
		title := options.Title
		if title == "" {
			title = "API reference"
		}
		assetURL := func(name string) string { return ui.RelativeURL(page, page+"/_assets/"+name) }
		initializer := ui.DocumentScript(page, documentPath, "redoc", `Redoc.init(presentation,{downloadDefinitionUrl:documentURL.href,disableGoogleFont:true,theme:{typography:{fontFamily:"system-ui, sans-serif",headings:{fontFamily:"system-ui, sans-serif"},code:{fontFamily:"monospace"}}}},document.getElementById("redoc"));`)
		body := `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>` + html.EscapeString(title) + `</title><link rel="icon" href="data:,"></head><body style="margin:0"><div id="redoc"></div><script src="` + html.EscapeString(assetURL("redoc.standalone.js")) + `"></script><script src="` + html.EscapeString(assetURL("init.js")) + `"></script><footer><a href="` + html.EscapeString(assetURL("LICENSE")) + `">ReDoc license</a> <a href="` + html.EscapeString(assetURL("THIRD_PARTY_LICENSES.txt")) + `">Third-party licenses</a></footer></body></html>`
		return []boot.OpenAPIResource{
			{Path: page, ContentType: "text/html; charset=utf-8", Body: body},
			ui.Asset(page, "redoc.standalone.js", "text/javascript; charset=utf-8", offlineJavascript),
			ui.Asset(page, "init.js", "text/javascript; charset=utf-8", initializer),
			ui.Asset(page, "LICENSE", "text/plain; charset=utf-8", license),
			ui.Asset(page, "redoc.standalone.js.LICENSE.txt", "text/plain; charset=utf-8", bundleLicenses),
			ui.Asset(page, "THIRD_PARTY_LICENSES.txt", "text/plain; charset=utf-8", thirdPartyLicenses),
		}, nil
	}
}
