// Package ui는 선택 UI 패키지의 공통 정적 리소스 구성을 돕습니다.
package ui

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/NARUBROWN/spine/internal/configuration"
	"github.com/NARUBROWN/spine/internal/openapi/resourcepath"
)

// RelativeURL은 프록시가 덧붙인 외부 접두사도 유지하는 상대 URL을 만듭니다.
func RelativeURL(page, target string) string {
	base := strings.Split(strings.Trim(path.Dir(page), "/"), "/")
	if len(base) == 1 && base[0] == "" {
		base = nil
	}
	dest := strings.Split(strings.TrimPrefix(target, "/"), "/")
	for len(base) > 0 && len(dest) > 1 && base[0] == dest[0] {
		base, dest = base[1:], dest[1:]
	}
	relative := strings.Repeat("../", len(base)) + strings.Join(dest, "/")
	if relative == "" {
		return "./"
	}
	return relative
}

// JSString은 HTML 안에서도 안전하게 사용할 JSON 문자열을 만듭니다.
func JSString(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}

// presentationScript changes only the UI copy, leaving the HTTP document unchanged.
const presentationScript = `
const presentation = structuredClone(spec);
const compact = (value) => {
  if (!value || typeof value !== "object") return;
  if (value["x-spine-type-range"] === true) {
    delete value.minimum;
    delete value.maximum;
  }
  if (value.example !== undefined && Array.isArray(value.examples)) delete value.examples;
  for (const [key, nested] of Object.entries(value)) {
    if (key === "properties" && nested && typeof nested === "object") {
      for (const property of Object.values(nested)) {
        compact(property);
        const range = typeRange(property);
        if (range) property.description = [property.description, "Type range: " + range].filter(Boolean).join("\n\n");
      }
    } else if (!["example", "examples", "default", "enum", "const"].includes(key)) {
      compact(nested);
    }
  }
};
const typeRange = (schema) => {
  if (!schema || typeof schema !== "object") return "";
  if (schema["x-spine-type-range"] === true && typeof schema["x-spine-type-range-text"] === "string") return schema["x-spine-type-range-text"];
  for (const nested of [...(schema.allOf || []), ...(schema.anyOf || [])]) {
    const range = typeRange(nested);
    if (range) return range;
  }
  return "";
};
compact(presentation);
// Simplify only the intersection of a nullable scalar and an explicit not-null rule.
// Unknown keywords or conflicting constraints leave the original schema intact.
const compactRequiredScalar = (schema) => {
  const result = {};
  let sawNullable = false, excludesNull = false;
  const annotations = new Set(["description", "example", "examples", "title", "deprecated", "readOnly", "writeOnly"]);
  const scalarKeys = new Set(["type", "format", "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "minLength", "maxLength", "pattern", "multipleOf"]);
  const visit = (node) => {
    if (!node || typeof node !== "object" || Array.isArray(node)) return false;
    for (const [key, value] of Object.entries(node)) {
      if (key === "allOf") {
        if (!Array.isArray(value) || !value.every(visit)) return false;
      } else if (key === "anyOf") {
        if (!Array.isArray(value) || value.length !== 2) return false;
        const nullBranch = value.find(branch => branch && Object.keys(branch).length === 1 && branch.type === "null");
        const nonNull = value.filter(branch => branch !== nullBranch);
        if (!nullBranch || nonNull.length !== 1 || !visit(nonNull[0])) return false;
        sawNullable = true;
      } else if (key === "not") {
        if (!value || value.type !== "null" || Object.keys(value).length !== 1) return false;
        excludesNull = true;
      } else if (annotations.has(key)) {
        if (!(key in result)) result[key] = value;
      } else if (scalarKeys.has(key)) {
        if (key in result && result[key] !== value) return false;
        result[key] = value;
      } else if (!key.startsWith("x-spine-")) {
        return false;
      }
    }
    return true;
  };
  if (!visit(schema) || !sawNullable || !excludesNull || !["integer", "number", "string", "boolean"].includes(result.type)) return null;
  return result;
};
for (const item of Object.values(presentation.paths || {})) {
  for (const operation of Object.values(item || {})) {
    if (!operation || typeof operation !== "object" || !Array.isArray(operation.parameters)) continue;
    for (const parameter of operation.parameters) {
      let schema = parameter.schema;
      if (!schema || typeof schema !== "object") continue;
      if (parameter.in === "query" && parameter.required) {
        const compacted = compactRequiredScalar(schema);
        if (compacted) schema = parameter.schema = compacted;
      }
      const notes = [];
      const collect = (current) => {
        if (!current || typeof current !== "object") return;
        for (const [key, label] of [["exclusiveMinimum", ">"], ["minimum", ">="], ["exclusiveMaximum", "<"], ["maximum", "<="]]) {
          const number = current[key];
          if (typeof number === "number" && Number.isFinite(number) && (!Number.isInteger(number) || Number.isSafeInteger(number))) {
            notes.push(label + " " + number);
          }
        }
        if (Array.isArray(current.allOf)) for (const nested of current.allOf) collect(nested);
      };
      collect(schema);
      const parts = [parameter.description || schema.description || "", notes.length ? "Constraints: " + [...new Set(notes)].join(", ") : ""].filter(Boolean);
      if (parts.length) parameter.description = parts.join("\n\n");
    }
  }
}
`

// DocumentScript는 명세를 읽고 외부 프록시 접두사를 기본 서버 주소에 반영합니다.
// 명세에 서버가 명시된 경우에는 그 선언을 그대로 사용합니다.
func DocumentScript(page, document, container, initialize string) string {
	return `(async()=>{const documentURL=new URL(` + JSString(RelativeURL(page, document)) + `,window.location.href);const response=await fetch(documentURL);if(!response.ok)throw new Error("HTTP "+response.status);const spec=await response.json();` + presentationScript + `if(!spec.servers||spec.servers.length===0){const documentPath=new URL(` + JSString(document) + `,documentURL.origin).pathname;const prefix=documentURL.pathname.slice(0,-documentPath.length);spec.servers=[{url:documentURL.origin+prefix}];}presentation.servers=spec.servers;` + initialize + `})().catch(error=>{document.getElementById(` + JSString(container) + `).textContent="Failed to load the OpenAPI document: "+error.message;});`
}

func ValidatePaths(page, document string) error {
	if !resourcepath.Valid(page) || page == "/" || !resourcepath.Valid(document) {
		return fmt.Errorf("Documentation UI and document paths must be static absolute paths, such as /docs and /openapi.json")
	}
	return nil
}

func Asset(page, name, contentType, body string) configuration.OpenAPIResource {
	return configuration.OpenAPIResource{Path: page + "/_assets/" + name, ContentType: contentType, Body: body}
}
