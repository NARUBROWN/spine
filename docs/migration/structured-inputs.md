# 쿼리와 헤더 구조체 입력으로 전환하기

이 개발 변경에서는 `query.Values`, `header.Values`와 각 타입의 조회 API 및 `NewValues` 생성 함수를 제거합니다. 쿼리와 헤더를 각각 일반 구조체 포인터로 받고 필드에 직접 접근하세요. 필드마다 함수 인자를 추가하거나 문서용 래퍼를 만들 필요가 없습니다.

현재 안내는 작업 브랜치의 변경을 대상으로 합니다. 전환을 포함한 정식 릴리스 버전은 아직 정해지지 않았습니다. 이전 공개 버전의 동작이 소급 변경되는 것은 아닙니다.

## 이전 코드를 찾기

```sh
rg -n 'query\.Values|header\.Values|query\.NewValues|header\.NewValues|pkg/header' --glob '*.go'
```

구버전에서는 `query.Values`와 `header.Values`를 주입받은 뒤 함수 내부에서 이름으로 조회했습니다. 다음은 구버전 전용 코드입니다.

```go
func (c *Controller) Search(q query.Values, headers header.Values) httpx.Response[map[string]any] {
    keyword := q.Get("q")
    page := q.Int("page", 1)
    requestID := headers.Get("X-Request-ID")
    return httpx.Response[map[string]any]{Body: map[string]any{
        "keyword": keyword, "page": page, "request_id": requestID,
    }}
}
```

## 출처별 구조체와 핸들러 인자 만들기

다음은 새 입력 구조를 보여주는 선언 예입니다. 핸들러 예제는 앱의 Controller와 pkg/httpx를 사용합니다.

```go
type SearchQuery struct {
    Keyword string `query:"q" validate:"required"`
    Page    *int   `query:"page" validate:"omitempty,gte=1"`
}

type RequestHeaders struct {
    RequestID string `header:"X-Request-ID" validate:"required"`
}
```

새 핸들러는 같은 반환형을 유지하면서 입력 필드에 직접 접근합니다.

```go
func (c *Controller) Search(q *SearchQuery, headers *RequestHeaders) httpx.Response[map[string]any] {
    page := 1
    if q.Page != nil {
        page = *q.Page
    }
    return httpx.Response[map[string]any]{Body: map[string]any{
        "keyword": q.Keyword, "page": page, "request_id": headers.RequestID,
    }}
}
```

핸들러는 `q *SearchQuery`, `headers *RequestHeaders`를 별도 인자로 받습니다. 함수 안에서는 `q.Keyword`, `q.Page`, `headers.RequestID`를 읽습니다. 본문이 필요하면 기존 본문 DTO도 별도 인자로 받습니다. 라우트의 메서드, 경로와 핸들러 등록 방식은 유지합니다.

`query` 태그가 있는 구조체는 HTTP 메서드와 무관하게 쿼리에서만 바인딩합니다. `header` 태그가 있는 구조체는 헤더에서만 바인딩합니다. JSON 본문이 이 값을 덮어쓰지 않습니다. 쿼리와 헤더 DTO에 다른 query, header, form, param 입력 출처를 섞으면 사전 검사에서 `HTTP_INPUT_CONTRACT_INVALID`로 거부합니다. 해당 출처의 태그가 없는 공개 필드는 조용히 무시하지 않습니다. 제외할 필드는 `query:"-"` 또는 `header:"-"`로 명시하세요. `headers`, `queries` 같은 지원하지 않는 복수형 입력 태그도 거부합니다. `json` 태그는 명명이나 직렬화에 사용할 수 있지만 출처를 다시 본문으로 바꾸지 않습니다.

본문 DTO의 query, header, param 입력 태그도 사전 검사에서 거부합니다. 경로 값은 기존 `path.Int` 같은 별도 경로 인자로 받으세요.

기존 DTO에 `json`과 `query` 태그를 함께 넣고 플랫폼의 병합 바인딩에 의존했다면, 쿼리 DTO와 본문 DTO를 분리하세요. 특히 이전 Echo 바인딩의 GET, DELETE 제한과 본문 덮어쓰기 동작은 새 쿼리 구조체에 적용하지 않습니다.

## 같은 DTO를 중복 인자로 선언하지 않기

한 핸들러에서 같은 구조체 타입의 입력 DTO를 두 번 이상 선언하면 `HTTP_INPUT_DTO_DUPLICATED`로 거부합니다. 예를 들어 `Create(first *CreateBody, second *CreateBody)`는 허용하지 않습니다. 인자 이름을 바꾸어도 다른 입력으로 취급하지 않습니다. 필요한 값은 기존 인자에서 읽으세요.

타입 별칭인 `type Alias = CreateBody`도 원래 타입과 같으므로 중복으로 판정합니다. 서로 다른 패키지의 동명 타입과 `type OtherBody CreateBody`처럼 새로 정의한 구조체 타입은 이 중복 타입 검사에서 구분합니다. 필드 모양이나 타입 이름 문자열만 비교하지 않습니다.

검사는 핸들러마다 독립적으로 수행합니다. 같은 DTO를 다른 핸들러나 라우트에서 사용하는 것은 허용합니다. context.Context와 ControllerContext 같은 컨텍스트, 서로 다른 경로 키를 받는 path.Int 같은 내장 경로 인자는 DTO 중복 검사 대상이 아닙니다.

Validate, Prepare와 Run은 생성자, 검증기, 어댑터 초기화와 본문 읽기 전에 이 설정 오류를 반환합니다. HTTP를 활성화한 DescribeHTTP도 같은 검사를 사용합니다. OpenAPI 리소스를 Disabled로 설정해도 앱 사전 검사의 중복 규칙은 유지됩니다. 오류의 Routes[i].Parameters[j]는 receiver를 제외한 0부터 시작하는 인자 위치입니다.

본문 DTO는 타입이 달라도 하나만 허용합니다. JSON DTO와 폼 DTO를 각각 받는 경우도 `HTTP_BODY_PARAMETERS_DUPLICATED`로 부팅 전에 거부합니다.

본문 DTO 내부에서 JSON 이름이 같은 깊이에서 모호하게 겹치거나 폼 입력 키가 중복되어도 `HTTP_INPUT_CONTRACT_INVALID`로 거부합니다. 정상적인 JSON 익명 임베딩의 필드 가리기 규칙은 유지합니다. json 태그로 제외한 중첩 구조체도 다른 본문 형식의 바인더가 읽을 수 있으므로 query, header, param 태그 혼합 검사에서 제외하지 않습니다.

서로 다른 쿼리 DTO나 헤더 DTO가 같은 입력 키를 선언하면 `HTTP_INPUT_KEY_DUPLICATED`로 거부합니다. 헤더 이름은 대소문자를 구분하지 않습니다. `query.Pagination`의 page, size도 같은 검사에 포함합니다. 본문, 쿼리, 헤더처럼 출처가 다른 동일 이름은 충돌로 취급하지 않습니다. 이 검사는 validator 등록 여부와 OpenAPI 활성화 여부에 관계없이 적용합니다.

본문의 json 태그와 검증 태그는 필수 조건이 아닙니다. 태그 없는 본문 필드는 Go JSON 기본 필드명을 그대로 사용합니다. 검증 태그를 요구하지 않으며, 선택한 validator가 있으면 실제 설정된 규칙만 적용합니다.

## 누락, 기본값과 변환 오류 구분하기

| 입력 상태 | 새 구조체 바인딩 |
| --- | --- |
| 문자열 누락 | 빈 문자열 |
| 포인터 필드 누락 | nil |
| 숫자에 잘못된 문자열 전달 | HTTP 400, 기본값으로 대체하지 않음 |
| 정수 범위 초과 | HTTP 400 |
| 등록한 validator의 required 위반 | 핸들러 실행 전 HTTP 400 |
| validator 미등록 | 변환은 수행하고 검증 태그는 적용하지 않음 |

기존 `q.Int("page", 1)`은 누락과 변환 실패 모두에 1을 반환했습니다. 같은 코드를 기계적으로 숫자 필드로 바꾸면 동작이 달라집니다. 누락에만 기본값을 적용하려면 위 예처럼 포인터로 받고 nil일 때 앱의 기본값을 사용하세요. `query.Pagination`은 별도 내장 입력으로 유지되며 기존 page 1, size 20 동작을 유지합니다.

숫자와 bool의 `required`는 선택한 validator의 규칙을 따릅니다. go-playground 구현에서 0과 false를 허용하면서 누락만 거부하려면 `*int`, `*bool` 필드와 `validate:"required"`를 사용하세요. 존재 여부만 확인하던 `Has`도 포인터 필드로 전환할 수 있습니다.

## JSON null과 Host 바인딩

일반 값 타입에 JSON `null`을 보내면 0, false나 빈 문자열로 대체하지 않고 입력 오류로 거부합니다. HTTP에서는 400을 반환하며 validator와 핸들러를 실행하지 않습니다. WS와 이벤트 소비자의 JSON DTO 디코딩도 같은 타입 규칙을 사용합니다. 일반 구조체 전체와 배열 전체의 null, 중첩 필드, 배열과 맵 원소에도 적용합니다.

필드 누락은 이 검사로 거부하지 않습니다. `*int`, `*bool`, `*string` 같은 포인터와 map, slice의 null은 nil로 유지합니다. 그 뒤 등록한 validator가 null을 허용하는지 판단합니다. 예를 들어 go-playground 구현에서 `*int`와 `validate:"required"`를 사용하면 null은 실패하고 실제 숫자 0은 통과합니다. `json.Unmarshaler`를 구현한 타입은 자신의 디코더에서 null 처리를 결정합니다.

`header:"Host"` 필드는 Go 요청의 `Request.Host`에서 읽습니다. 포트가 있으면 그대로 전달하며 `X-Forwarded-Host`로 대체하지 않습니다. 일반 Header 조회와 Headers 조회 뷰에도 같은 값을 제공합니다. 원래 요청 헤더 맵은 변경하지 않습니다. DTO 선언과 필드 접근 방식은 바뀌지 않습니다.

HTTP 수정은 이 규칙을 적용한 Echo 플랫폼과 코어를 함께 사용해야 합니다. 기존 공개 Echo 플랫폼 `v0.1.0`에는 포함되지 않습니다.

## 지원하는 필드와 검증기

태그가 있는 공개 필드만 바인딩합니다. 스칼라, 스칼라 포인터와 슬라이스, RFC3339의 time.Time, encoding.TextUnmarshaler를 지원합니다. 쿼리 슬라이스는 반복 키와 쉼표 구분을 함께 읽습니다. 헤더 이름은 대소문자를 구분하지 않으며, 헤더 슬라이스는 지원하는 목록 문법으로 반복 값과 쉼표 목록을 읽습니다. 사용자 TextUnmarshaler는 요청 중에만 실행합니다. map, 일반 중첩 객체, 익명 포인터 임베딩과 중복 이름은 지원하지 않습니다. 공개 익명 값 구조체의 필드는 펼쳐서 바인딩합니다.

기존 `spine.WithValidator(...)` 연결을 유지하세요. 변환 실패 시 validator를 호출하지 않습니다. 검증기는 쿼리와 헤더 구조체를 각각 검증하며 사용자 규칙도 기존 계약을 따릅니다. Playground 개발 변경은 요청에서 선택한 실제 출처의 태그를 오류 이름에 사용합니다. 요청별 선택이 없는 호출은 생성 시 지정한 WithFieldTags 기본값을 유지합니다.

query 또는 header에서 `"-"`로 제외한 필드를 선택한 validator가 필수로 요구하면 `HTTP_INPUT_VALIDATION_CONFLICT`로 부팅 전에 거부합니다. 검증 태그를 강제하지 않으며 validator가 없으면 이 규칙도 적용하지 않습니다. 이 검사는 SchemaDescriber가 설명한 필수 제약에 적용하며 사용자 규칙의 실행 결과를 추측하지 않습니다. OpenAPI를 꺼도 같은 검사를 수행합니다.

기본 Echo 본문 바인딩은 경로와 URL 쿼리를 병합하지 않습니다. urlencoded 폼도 본문 값만 사용합니다. 익명 임베딩과 태그 없는 중첩 값 구조체 안의 form 필드도 같은 필드 계획으로 판별하고 명세에 펼칩니다. 사용자 지정 Binder는 해당 구현의 바인딩 규칙을 따릅니다.

## 쿼리 슬라이스와 반복 입력

`Tags []string`에 `query:"tags"`를 선언하면 반복 키와 쉼표 구분을 모두 허용합니다. 두 형식을 섞어도 요청 순서와 중복 값을 유지합니다. 공백과 빈 문자열 원소는 제거하지 않습니다.

| 요청 | 바인딩 결과 |
| --- | --- |
| `?tags=a&tags=b` | `["a", "b"]` |
| `?tags=a,b` | `["a", "b"]` |
| `?tags=a,b&tags=c` | `["a", "b", "c"]` |
| `?tags=a%2Cb` | `["a,b"]` |
| `?tags=a,%20b` | `["a", " b"]` |
| `?tags=a&tags=a` | `["a", "a"]` |
| `?tags=,` | `["", ""]` |

구분자 쉼표로 나눈 뒤 각 원소를 URL 디코딩하므로 `%2C`는 원소 안의 쉼표로 유지합니다. `%252C`는 한 번만 디코딩해 `%2C`로 유지합니다. 숫자 슬라이스도 같은 방식으로 나눈 뒤 변환하며 원소 하나라도 변환할 수 없으면 validator 실행 전에 400을 반환합니다. 예를 들어 `%20` 공백이 붙은 숫자는 자동으로 다듬지 않습니다.

일반 문자열 필드는 쉼표를 그대로 받습니다. 필드 전체에 `TextUnmarshaler`를 구현한 사용자 타입도 하나의 입력값을 자신의 디코더에 전달합니다. 각 원소가 TextUnmarshaler를 구현한 슬라이스는 목록을 나눈 뒤 원소별로 디코딩합니다.

자동 바인딩하는 단일 값 쿼리에 같은 키를 여러 번 보내면 값이 같아도 400을 반환합니다. 포인터 필드도 같은 규칙을 따릅니다. 내장 `query.Pagination`의 page와 size에도 적용하며, 누락 시 기본값은 page 1과 size 20을 유지합니다. 이 개수 검사는 validator 등록 여부와 무관합니다.

쉼표 구분에는 선택적 HTTP 계약인 `RawQueryContext`를 제공하는 플랫폼이 필요합니다. 이 변경을 포함한 Echo 플랫폼은 원시 쿼리를 제공합니다. 기존 플랫폼이 디코딩된 Queries만 제공하면 반복 키 바인딩을 유지하고, 이미 디코딩된 쉼표를 구분자로 추측하지 않습니다. 자동 OpenAPI 명세의 대표 표현은 기존 `style: form`, `explode: true`인 반복 키 방식으로 유지합니다.

## 헤더 중복과 목록

단일값 헤더의 반복 입력은 값이 같아도 400으로 거부합니다. `Accept-Encoding` 같은 목록형 헤더는 여러 줄을 순서대로 병합해 문자열 필드에 전달합니다. 일반 슬라이스 필드는 목록 원소로 나누고, 따옴표 안의 쉼표를 보존합니다. `If-Match`의 `"a,b", "c"`는 원소 두 개로 처리하며 각 원소의 따옴표 표현은 유지합니다.

목록 파서는 Link의 URI와 Via의 주석 안에 있는 쉼표도 보호합니다. 구분자 주변의 SP와 HTAB만 문법 공백으로 처리하고 원소 내부의 표현은 유지합니다. 사용자 정의 헤더는 scalar 선언이면 단일값, 일반 슬라이스 선언이면 쉼표 목록 계약으로 취급합니다. 필드 전체의 TextUnmarshaler는 자체 파싱을 위해 병합한 전체 값을 받습니다.

Host, Content-Type, Authorization, Cookie 같은 단일값 또는 별도 문법의 헤더에 일반 슬라이스를 선언하면 시작 전에 거부합니다. WWW-Authenticate처럼 별도 목록 문법이 필요한 필드도 문자열이나 자체 TextUnmarshaler로 받아야 합니다. Cookie의 여러 줄은 쉼표 대신 `; `로 병합합니다. 자세한 규칙은 [HTTP 어댑터 계약](../../contract/http/README.md)을 참고하세요.

## 오류의 출처와 필드명

검증 오류의 각 필드에는 body, query, header 중 하나인 `in`을 덧붙입니다. 이름은 실제 바인딩에 사용한 json, form, xml, query 또는 header 태그를 따릅니다. 헤더 DTO에 직렬화용 json 태그가 함께 있어도 검증 오류에는 header 태그의 이름을 사용합니다.

```json
{"message":"Validation failed","fields":[{"in":"header","field":"/X-ID","rule":"required"}]}
```

바인딩 오류에는 최상위 `in`을 표시하며 validator를 실행하지 않습니다. 검증기 처리 실패와 취소는 기존 오류 구분을 유지합니다. 원래 공통 검증 오류와 오류 체인은 변경하지 않으며 HTTP 응답에만 출처를 추가합니다.

## 명세와 전환 확인

validator가 없으면 OpenAPI는 경로와 HTTP 메서드만 설명합니다. 입력 모델, 쿼리와 헤더 정보, 반환 모델과 오류 상태 코드를 추정하지 않습니다. 표준 OpenAPI 구조에 필요한 최소 응답 자리와 타입을 지정하지 않은 경로 변수 자리만 남깁니다. 태그 없는 본문 바인딩은 계속 사용할 수 있습니다.

validator가 있으면 해당 구현의 SchemaDescriber로 상세 입력을 설명합니다. 검증기는 있지만 설명 계약이 없으면 `INPUT_SCHEMA_DESCRIPTION_UNAVAILABLE`로 상세 입력 생략 이유를 남깁니다. validator 자체가 없으면 `VALIDATOR_NOT_CONFIGURED`로 경로만 설명하는 이유를 남깁니다.

JSON 요청의 고정 배열은 Go 디코딩처럼 부족한 원소를 기본값으로 채우고 길이를 넘는 원소를 버립니다. 요청 명세는 처리할 위치만 prefixItems로 설명하고, 응답은 고정 길이 배열로 설명합니다. 실제 배열 원소에 적용한 검증 제약은 유지합니다. byte slice 요청은 base64 문자열과 숫자 배열을 모두 설명하며 응답은 base64 문자열로 설명합니다. 익명 구조체의 스키마 식별에는 필드 타입의 전체 패키지 경로를 포함합니다.

명세 리소스는 기본 공개입니다. 부팅 옵션의 `HTTP.OpenAPI.Disabled`를 true로 설정하면 자동 생성과 HTTP 제공을 끕니다. 문서 요청은 업무용 Spine 인터셉터를 실행하지 않습니다.

```sh
go test ./...
```

실제 HTTP 테스트에서는 잘못된 타입, 필수값 누락, 입력 키 중복과 출처별 오류를 확인하세요. validator를 생략해도 변환 오류와 부팅 시 설정 오류는 계속 거부해야 합니다. CLI 연동과 마이그레이션은 코어 입력 스펙 확정 후 별도 이슈로 진행합니다.
