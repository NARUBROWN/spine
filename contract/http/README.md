# HTTP 어댑터 실행 계약

`github.com/NARUBROWN/spine/contract/http`는 Spine 커널과 플랫폼 구현이 공유하는 공개 패키지입니다. Spine 모듈에 포함되며 버전도 Spine과 함께 관리합니다. 별도 계약 모듈이나 저장소는 사용하지 않습니다.

계약 패키지는 표준 라이브러리만 import합니다. Spine의 내부 파이프라인, 요청 store, 이벤트 버스와 구체 HTTP 프레임워크에 의존하지 않습니다. 외부 플랫폼 모듈은 이 공개 패키지를 import하여 Factory와 HTTPAdapter를 구현할 수 있습니다.

Factory는 기능을 선언하고 리스너 없이 새 어댑터를 구성합니다. HTTPContext는 요청 입력과 실제 ResponseWriter를 제공하며, ExecuteFunc는 요청마다 커널을 한 번 동기 실행합니다. Handler는 외부 HTTP 서버에 연결할 수 있고 Serve와 Shutdown은 관리 서버 실행에 사용합니다.

현재 계약 버전은 `spine.http-adapter/v2`입니다. `ResponseWriter.WriteJSON`은 core에서 직렬화한 `json.RawMessage`만 받습니다. DTO의 동적 타입이 필요한 `HTTPContext.Bind(any)`가 공개 HTTP 계약에 남는 유일한 `any`입니다.

native Echo 설정은 플랫폼의 `WithEcho(func(*echo.Echo))` 옵션이 소유합니다. 공통 Config와 Capabilities에는 native hook을 넣지 않습니다. v1 구현은 버전 검사에서 거부하므로 커널과 플랫폼을 함께 전환해야 합니다.

scope와 writer는 요청 중에만 사용하며 별도 goroutine에서 동시 접근하거나 보관하지 않습니다. Spine의 요청 store, 라우팅 결과, EventBus와 응답 staging은 커널이 소유합니다. 요청 오류는 RequestError로 상태와 원인을 전달하고 커널이 공개 HTTP 오류로 변환합니다.

## Host와 JSON 입력 바인딩

이 작업 브랜치에서는 Header와 Headers의 Host 조회에 Go 요청의 `Request.Host`를 사용합니다. Go가 Host를 헤더 맵과 분리하므로 플랫폼이 별도로 보충해야 합니다. 대소문자가 다른 Host 이름도 같은 값을 읽으며 포트가 있으면 그대로 보존합니다. `X-Forwarded-Host` 같은 전달 헤더로 대체하지 않습니다. Headers는 원래 요청 맵을 변경하지 않는 조회 뷰를 반환합니다.

기본 JSON 입력 바인딩은 `int`, `bool`, `string`, 일반 구조체와 배열처럼 nil을 표현할 수 없는 타입에 명시한 `null`을 거부합니다. 중첩 필드와 컬렉션 원소에도 같은 규칙을 적용합니다. 필드 누락은 그대로 두며 포인터, map, slice와 interface의 null은 허용합니다. 필수 여부와 값 제약은 등록한 validator가 판단합니다.

플랫폼은 공통 `github.com/NARUBROWN/spine/pkg/jsonx.Unmarshal`을 사용해 이 규칙을 적용할 수 있습니다. 이 함수는 JSON 필드 이름, 익명 필드 선택과 사용자 정의 `json.Unmarshaler`를 보존합니다. 사용자 정의 JSON 디코더의 null 처리는 해당 구현이 결정합니다. null 타입 오류는 `RequestError`로 감싼 HTTP 400으로 전달하고 validator나 핸들러를 실행하지 않습니다.

이 동작은 새 플랫폼 구현과 코어를 함께 적용해야 합니다. 기존에 공개한 Echo 플랫폼 `v0.1.0`에 소급 적용되지 않습니다.

## 원시 쿼리 조회

선택적 `RawQueryContext`는 `RawQuery() (string, bool)`로 URL 디코딩 전의 쿼리를 제공합니다. 값은 `Request.URL.RawQuery`처럼 선행 물음표가 없는 문자열입니다. bool은 제공 여부이며, 빈 쿼리도 제공할 수 있으면 true를 반환합니다. 기본 HTTPContext의 필수 메서드와 계약 버전은 바꾸지 않습니다.

코어는 원시 쿼리에서 반복 키와 슬라이스의 쉼표 구분을 처리한 뒤 원소별로 URL 디코딩합니다. 따라서 `tags=a,b`는 두 원소가 되고 `tags=a%2Cb`는 쉼표를 포함한 한 원소가 됩니다. 공백, 빈 원소와 중복 값은 보존합니다. 기존 Query와 Queries의 디코딩된 조회 뷰는 유지합니다.

원시 쿼리를 제공하지 않는 플랫폼에서는 반복 키 바인딩을 유지합니다. 디코딩된 값에서는 구분자 쉼표와 인코딩된 쉼표를 구별할 수 없으므로 쉼표 분할을 적용하지 않습니다. 새 쉼표 구분 동작을 사용하려면 플랫폼도 이 선택적 계약을 구현해야 합니다.

ASGI의 앱과 서버 경계 및 수명 정의를 참고하지만 **Python ASGI 호환을 제공하지 않습니다.** Spine v0.6.0과 Echo 플랫폼 v0.1.0에서 이 계약을 사용합니다.

```sh
go test ./contract/http -count=1
```

이전 `github.com/NARUBROWN/spine/adapter` 경로는 같은 타입과 오류의 호환 별칭을 제공합니다. 기존 Echo 플랫폼을 그대로 연결할 수 있으며 새 구현은 `contract/http`를 사용합니다.

## 헤더 개수와 목록 바인딩

단일값 헤더는 같은 값의 반복도 거부합니다. 지원하는 표준 목록형 헤더는 여러 줄을 순서대로 병합합니다. 문자열 필드는 전체 필드값을 받고 일반 슬라이스 필드는 파싱한 목록을 받습니다. 사용자 정의 헤더는 scalar와 일반 슬라이스의 선언으로 단일값 또는 쉼표 목록을 정합니다.

기본 목록 처리에는 Accept 계열, Allow, Cache-Control, Connection, Content-Encoding, Content-Language, Forwarded, Pragma, TE, Trailer, Transfer-Encoding, Upgrade, Vary, 접근 제어 목록과 WebSocket 프로토콜 목록을 포함합니다. If-Match와 If-None-Match의 opaque entity tag, Via의 주석과 Link의 URI도 별도로 처리합니다. 원소의 프로토콜 표현은 유지하고 구분자 주변의 SP와 HTAB만 정리합니다. [RFC 9110의 필드 결합 규칙](https://www.rfc-editor.org/rfc/rfc9110.html#section-5.3)을 따릅니다.

Host, Content-Type, Authorization과 날짜 헤더처럼 단일값인 필드에는 일반 슬라이스를 사용할 수 없습니다. Cookie와 인증 challenge 목록도 일반 쉼표 배열로 처리하지 않습니다. Cookie는 여러 줄을 `; `로 병합하고, WWW-Authenticate와 Proxy-Authenticate는 문자열이나 자체 TextUnmarshaler를 사용합니다. 필드 전체의 TextUnmarshaler는 자체 파싱을 위해 전체 값을 받습니다. [HTTP/2 Cookie 결합 규칙](https://www.rfc-editor.org/rfc/rfc9113.html#section-8.2.3)도 보존합니다.

이 타입과 개수 검사는 validator 유무와 관계없이 적용합니다. 태그 없는 본문은 Go JSON 기본 필드명으로 계속 바인딩할 수 있습니다.
