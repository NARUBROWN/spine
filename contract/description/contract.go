// Package description은 앱 등록을 읽는 공개 데이터 계약입니다.
// 생성자나 전송 구현을 포함하지 않으며 표준 라이브러리만 사용합니다.
package description

import "encoding/json"

const ContractVersion = "spine.http-description/v1alpha1"

// HTTP는 한 번의 설명 요청에서 복사한 HTTP 등록과 스키마입니다.
// 반환값을 수정해도 앱 등록이나 이미 준비된 서버의 명세는 바뀌지 않습니다.
type HTTP struct {
	ContractVersion string `json:"contract_version"`
	Enabled         bool   `json:"enabled"`
	// PathsOnly는 validator 미등록으로 경로와 메서드만 설명하는 모드입니다.
	PathsOnly   bool                       `json:"paths_only,omitempty"`
	Routes      []Route                    `json:"routes"`
	Schemas     map[string]json.RawMessage `json:"schemas"`
	Diagnostics []Diagnostic               `json:"diagnostics"`
}

type Route struct {
	Method      string   `json:"method"`
	Path        string   `json:"path"`
	Handler     string   `json:"handler"`
	Tags        []string `json:"tags,omitempty"`
	Summary     string   `json:"summary,omitempty"`
	Description string   `json:"description,omitempty"`
	Inputs      []Input  `json:"inputs"`
	Returns     []Return `json:"returns"`
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

// Registrations는 앱의 이벤트 소비자와 WebSocket 등록을 복사한 결과입니다.
type Registrations struct {
	Consumers  []ConsumerRegistration  `json:"consumers"`
	WebSockets []WebSocketRegistration `json:"websockets"`
}

type ConsumerRegistration struct {
	Topic   string  `json:"topic"`
	Handler Handler `json:"handler"`
}

type WebSocketRegistration struct {
	Path    string  `json:"path"`
	Handler Handler `json:"handler"`
}

// Handler는 등록된 메서드를 식별합니다.
type Handler struct {
	PackagePath string `json:"package_path"`
	Receiver    string `json:"receiver"`
	Method      string `json:"method"`
}
