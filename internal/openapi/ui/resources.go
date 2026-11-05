// Package ui는 선택 UI 패키지의 공통 정적 리소스 구성을 돕습니다.
package ui

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/NARUBROWN/spine/pkg/boot"
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

// DocumentScript는 명세를 읽고 외부 프록시 접두사를 기본 서버 주소에 반영합니다.
// 명세에 서버가 명시된 경우에는 그 선언을 그대로 사용합니다.
func DocumentScript(page, document, container, initialize string) string {
	return `(async()=>{const documentURL=new URL(` + JSString(RelativeURL(page, document)) + `,window.location.href);const response=await fetch(documentURL);if(!response.ok)throw new Error("HTTP "+response.status);const spec=await response.json();if(!spec.servers||spec.servers.length===0){const documentPath=new URL(` + JSString(document) + `,documentURL.origin).pathname;const prefix=documentURL.pathname.slice(0,-documentPath.length);spec.servers=[{url:documentURL.origin+prefix}];}` + initialize + `})().catch(error=>{document.getElementById(` + JSString(container) + `).textContent="Failed to load the OpenAPI document: "+error.message;});`
}

func ValidatePaths(page, document string) error {
	if !boot.IsOpenAPIResourcePath(page) || page == "/" || !boot.IsOpenAPIResourcePath(document) {
		return fmt.Errorf("Documentation UI and document paths must be static absolute paths, such as /docs and /openapi.json")
	}
	return nil
}

func Asset(page, name, contentType, body string) boot.OpenAPIResource {
	return boot.OpenAPIResource{Path: page + "/_assets/" + name, ContentType: contentType, Body: body}
}
