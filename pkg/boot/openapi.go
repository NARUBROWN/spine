package boot

import "github.com/NARUBROWN/spine/internal/configuration"

// OpenAPIOptions는 명시적으로 활성화하는 HTTP 명세 리소스의 공개 설정입니다.
type OpenAPIOptions = configuration.OpenAPIOptions

// OpenAPIResource는 GET과 HEAD로 제공하는 불변 문서 리소스입니다.
type OpenAPIResource = configuration.OpenAPIResource

// OpenAPIUI는 명세 경로에 연결할 정적 리소스를 생성하는 공개 콜백입니다.
// 외부 자원을 열거나 사용자 핸들러를 실행하면 안 됩니다.
type OpenAPIUI = configuration.OpenAPIUI
