// Package swaggerui는 Swagger UI와 재배포 고지를 단일 바이너리에 포함합니다.
package swaggerui

import (
	_ "embed"
	"html"

	"github.com/NARUBROWN/spine/internal/openapi/ui"
	"github.com/NARUBROWN/spine/pkg/boot"
)

//go:embed assets/swagger-ui-bundle.js
var javascript string

//go:embed assets/swagger-ui.css
var stylesheet string

//go:embed assets/LICENSE
var license string

//go:embed assets/NOTICE
var notice string

//go:embed assets/THIRD_PARTY_LICENSES.txt
var thirdPartyLicenses string

//go:embed assets/swagger-ui-bundle.js.LICENSE.txt
var bundleLicenses string

// Options는 Swagger UI의 경로와 브라우저 제목을 설정합니다.
type Options struct {
	// Path를 생략하면 /docs를 사용합니다. GlobalPrefix와 독립적입니다.
	Path string
	// Title을 생략하면 API documentation을 사용합니다.
	Title string
}

// New는 네트워크 접근 없이 임베드한 화면과 정적 파일을 연결합니다.
// 반환값을 boot.OpenAPIOptions.UI에 넣고 Enabled를 true로 설정하세요.
func New(options Options) boot.OpenAPIUI {
	return func(documentPath string) ([]boot.OpenAPIResource, error) {
		page := options.Path
		if page == "" {
			page = "/docs"
		}
		if err := ui.ValidatePaths(page, documentPath); err != nil {
			return nil, err
		}
		title := options.Title
		if title == "" {
			title = "API documentation"
		}
		assetURL := func(name string) string { return ui.RelativeURL(page, page+"/_assets/"+name) }
		initializer := ui.DocumentScript(page, documentPath, "swagger-ui", `SwaggerUIBundle({url:documentURL.href,spec:spec,dom_id:"#swagger-ui",deepLinking:true,validatorUrl:null,persistAuthorization:false});`)
		body := `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>` + html.EscapeString(title) + `</title><link rel="icon" href="data:,"><link rel="stylesheet" href="` + html.EscapeString(assetURL("swagger-ui.css")) + `"></head><body><div id="swagger-ui"></div><script src="` + html.EscapeString(assetURL("swagger-ui-bundle.js")) + `"></script><script src="` + html.EscapeString(assetURL("init.js")) + `"></script><footer><a href="` + html.EscapeString(assetURL("LICENSE")) + `">Swagger UI license</a> <a href="` + html.EscapeString(assetURL("NOTICE")) + `">Notices</a> <a href="` + html.EscapeString(assetURL("THIRD_PARTY_LICENSES.txt")) + `">Third-party licenses</a></footer></body></html>`
		return []boot.OpenAPIResource{
			{Path: page, ContentType: "text/html; charset=utf-8", Body: body},
			ui.Asset(page, "swagger-ui-bundle.js", "text/javascript; charset=utf-8", javascript),
			ui.Asset(page, "swagger-ui.css", "text/css; charset=utf-8", stylesheet),
			ui.Asset(page, "init.js", "text/javascript; charset=utf-8", initializer),
			ui.Asset(page, "LICENSE", "text/plain; charset=utf-8", license),
			ui.Asset(page, "NOTICE", "text/plain; charset=utf-8", notice),
			ui.Asset(page, "swagger-ui-bundle.js.LICENSE.txt", "text/plain; charset=utf-8", bundleLicenses),
			ui.Asset(page, "THIRD_PARTY_LICENSES.txt", "text/plain; charset=utf-8", thirdPartyLicenses),
		}, nil
	}
}
