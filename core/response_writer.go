package core

import adapter "github.com/NARUBROWN/spine/contract/http"

// ResponseWriter는 전송 구현(Echo, net/http 등)에 의존하지 않는
// Spine의 응답 출력 계약이다.
// 실제 구현은 어댑터 계층에서 제공한다.
type ResponseWriter = adapter.ResponseWriter
