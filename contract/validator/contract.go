// Package validator는 Spine과 외부 검증 구현이 공유하는 DTO 검증 계약입니다.
// 표준 라이브러리만 사용하며 HTTP, WebSocket, 브로커와 구체 검증 라이브러리에 의존하지 않습니다.
package validator

import "context"

// ContractVersion은 이 패키지가 정의하는 검증 계약의 식별자입니다.
const ContractVersion = "spine.validation/v1"

// Validator는 역직렬화나 바인딩을 마친 DTO를 동기 검증합니다.
// value는 구조체 또는 nil이 아닌 구조체 포인터이며, 그 외의 값은 ErrInvalidValue로 감싼 오류를 반환합니다.
// 구현은 입력을 변경하거나 반환 후 보관하지 않으며, 한 인스턴스의 Validate를 동시에 호출할 수 있어야 합니다.
// 규칙 등록과 설정은 첫 호출 전에 마칩니다. 규칙의 이름과 필수값 판정 등은 구현이 문서화합니다.
type Validator interface {
	// Validate는 성공하면 nil, 규칙 위반이면 필드 오류가 하나 이상인 *Error를 반환합니다.
	// 호출자는 nil이 아닌 ctx를 전달합니다. 취소 시 ctx.Err()를 보존한 오류를 반환합니다.
	// 잘못된 설정과 검증 처리 실패는 *Error로 분류하지 않습니다.
	Validate(ctx context.Context, value any) error
}
