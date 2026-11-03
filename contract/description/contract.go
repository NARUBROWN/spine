// Package description은 실행된 등록을 HTTP 명세로 읽는 공개 데이터 계약입니다.
// 생성자나 전송 구현을 포함하지 않으며 표준 라이브러리만 사용합니다.
package description

import "encoding/json"

const ContractVersion = "spine.http-description/v1alpha1"

// HTTP는 한 번의 설명 요청에서 복사한 HTTP 등록과 스키마입니다.
// 반환값을 수정해도 앱 등록이나 이미 준비된 서버의 명세는 바뀌지 않습니다.
type HTTP struct {
	ContractVersion string                     `json:"contract_version"`
	Enabled         bool                       `json:"enabled"`
	Routes          []Route                    `json:"routes"`
	Schemas         map[string]json.RawMessage `json:"schemas"`
	Diagnostics     []Diagnostic               `json:"diagnostics"`
}

type Route struct {
	Method  string   `json:"method"`
	Path    string   `json:"path"`
	Handler string   `json:"handler"`
	Inputs  []Input  `json:"inputs"`
	Returns []Return `json:"returns"`
}

type Input struct {
	Index      int                        `json:"index"`
	Binding    string                     `json:"binding"`
	Name       string                     `json:"name,omitempty"`
	Type       string                     `json:"type"`
	Required   bool                       `json:"required,omitempty"`
	Schema     json.RawMessage            `json:"schema,omitempty"`
	Content    map[string]json.RawMessage `json:"content,omitempty"`
	Validation bool                       `json:"validation,omitempty"`
}

type Return struct {
	Type    string                     `json:"type"`
	Kind    string                     `json:"kind"`
	Content map[string]json.RawMessage `json:"content,omitempty"`
}

// Diagnostic은 명세가 추론하지 않은 정보를 구분합니다. 입력값은 포함하지 않습니다.
type Diagnostic struct {
	Code    string `json:"code"`
	Target  string `json:"target"`
	Message string `json:"message"`
}
