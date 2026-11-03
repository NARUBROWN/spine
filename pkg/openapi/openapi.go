// Package openapi는 런타임 HTTP 설명에서 표준 OpenAPI 문서를 생성합니다.
// CLI, 소스 파일과 외부 모듈 없이 사용할 수 있습니다.
package openapi

import (
	"encoding/json"

	"github.com/NARUBROWN/spine/contract/description"
	"github.com/NARUBROWN/spine/internal/apidesc"
	"github.com/NARUBROWN/spine/pkg/boot"
)

// Generate는 복사된 HTTP 설명에서 OpenAPI 3.1.2 JSON을 생성합니다.
func Generate(contract description.HTTP, options boot.OpenAPIOptions) (json.RawMessage, error) {
	return apidesc.OpenAPI(contract, options)
}
