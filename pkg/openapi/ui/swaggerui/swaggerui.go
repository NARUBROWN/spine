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

// conditionalPresentation은 Swagger UI가 조건부 필드를 항상 필수로 합치는 것을 방지합니다.
const conditionalPresentation = `
const schemas = presentation.components?.schemas || {};
const marker = "x-spine-conditional-required";
const schemaRefPrefix = "#/components/schemas/";
const conditionNotes = (schema, path = "", ancestors = new Set()) => {
  if (!schema || typeof schema !== "object" || ancestors.has(schema)) return [];
  if (schema[marker] === true && schema.if?.properties && Array.isArray(schema.then?.required)) {
    const condition = Object.entries(schema.if.properties).map(([field, value]) => path + field + " = " + JSON.stringify(value.const)).join(" and ");
    return [[condition, schema.then.required.map(field => path + field)]];
  }
  ancestors.add(schema);
  const notes = [];
  if (typeof schema.$ref === "string" && schema.$ref.startsWith(schemaRefPrefix)) {
    notes.push(...conditionNotes(schemas[schema.$ref.slice(schemaRefPrefix.length)], path, ancestors));
  }
  for (const key of ["allOf", "anyOf", "oneOf", "prefixItems"]) {
    if (!Array.isArray(schema[key])) continue;
    for (const child of schema[key]) notes.push(...conditionNotes(child, path + (key === "prefixItems" ? "[]." : ""), ancestors));
  }
  for (const [field, child] of Object.entries(schema.properties || {})) {
    notes.push(...conditionNotes(child, path + field + ".", ancestors));
  }
  notes.push(...conditionNotes(schema.items, path.replace(/\.$/, "") + "[].", ancestors));
  notes.push(...conditionNotes(schema.additionalProperties, path.replace(/\.$/, "") + "[key].", ancestors));
  ancestors.delete(schema);
  return notes;
};
for (const item of Object.values(presentation.paths || {})) {
  for (const operation of Object.values(item || {})) {
    if (!operation?.requestBody?.content) continue;
    const descriptions = [];
    for (const [media, content] of Object.entries(operation.requestBody.content)) {
      const grouped = new Map();
      for (const [condition, fields] of conditionNotes(content.schema)) {
        const required = grouped.get(condition) || new Set();
        for (const field of fields) required.add(field);
        grouped.set(condition, required);
      }
      for (const [condition, fields] of grouped) {
        descriptions.push("- " + media + ": " + condition + " → " + [...fields].join(", "));
      }
    }
    if (descriptions.length) {
      operation.description = ["**Conditionally required fields**\n\n" + descriptions.join("\n"), operation.description].filter(Boolean).join("\n\n");
    }
  }
}
for (const schema of Object.values(schemas)) {
  if (!Array.isArray(schema?.allOf)) continue;
  schema.allOf = schema.allOf.filter(branch => branch?.[marker] !== true);
  if (schema.allOf.length === 0) delete schema.allOf;
}
`

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
		initializer := ui.DocumentScript(page, documentPath, "swagger-ui", conditionalPresentation+`SwaggerUIBundle({url:documentURL.href,spec:presentation,dom_id:"#swagger-ui",deepLinking:true,validatorUrl:null,persistAuthorization:false});`)
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
