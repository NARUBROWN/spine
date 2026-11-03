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

`query` 태그가 있는 구조체는 HTTP 메서드와 무관하게 쿼리에서만 바인딩합니다. `header` 태그가 있는 구조체는 헤더에서만 바인딩합니다. JSON 본문이 이 값을 덮어쓰지 않습니다. 한 구조체에 query, header, form, param 입력 출처를 혼합하면 사전 검사에서 `HTTP_INPUT_CONTRACT_INVALID`로 거부합니다. `json` 태그는 명명이나 직렬화에 사용할 수 있지만 출처를 다시 본문으로 바꾸지 않습니다.

기존 DTO에 `json`과 `query` 태그를 함께 넣고 플랫폼의 병합 바인딩에 의존했다면, 쿼리 DTO와 본문 DTO를 분리하세요. 특히 이전 Echo 바인딩의 GET, DELETE 제한과 본문 덮어쓰기 동작은 새 쿼리 구조체에 적용하지 않습니다.

## 같은 DTO를 중복 인자로 선언하지 않기

한 핸들러에서 같은 구조체 타입의 입력 DTO를 두 번 이상 선언하면 `HTTP_INPUT_DTO_DUPLICATED`로 거부합니다. 예를 들어 `Create(first *CreateBody, second *CreateBody)`는 허용하지 않습니다. 인자 이름을 바꾸어도 다른 입력으로 취급하지 않습니다. 필요한 값은 기존 인자에서 읽으세요.

타입 별칭인 `type Alias = CreateBody`도 원래 타입과 같으므로 중복으로 판정합니다. 서로 다른 패키지의 동명 타입과 `type OtherBody CreateBody`처럼 새로 정의한 구조체 타입은 이 중복 타입 검사에서 구분합니다. 필드 모양이나 타입 이름 문자열만 비교하지 않습니다.

검사는 핸들러마다 독립적으로 수행합니다. 같은 DTO를 다른 핸들러나 라우트에서 사용하는 것은 허용합니다. context.Context와 ControllerContext 같은 컨텍스트, 서로 다른 경로 키를 받는 path.Int 같은 내장 경로 인자는 DTO 중복 검사 대상이 아닙니다.

Validate, Prepare와 Run은 생성자, 검증기, 어댑터 초기화와 본문 읽기 전에 이 설정 오류를 반환합니다. HTTP를 활성화한 DescribeHTTP도 같은 검사를 사용합니다. OpenAPI 리소스를 Disabled로 설정해도 앱 사전 검사의 중복 규칙은 유지됩니다. 오류의 Routes[i].Parameters[j]는 receiver를 제외한 0부터 시작하는 인자 위치입니다.

이 규칙은 같은 Go 타입의 중복 선언을 검사합니다. 서로 다른 타입을 사용하는 모든 조합의 유효성을 보장하는 규칙은 아닙니다.

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

태그가 있는 공개 필드만 바인딩합니다. 스칼라, 스칼라 포인터와 슬라이스, RFC3339의 time.Time, encoding.TextUnmarshaler를 지원합니다. 쿼리 슬라이스는 반복 키와 쉼표 구분을 함께 읽습니다. 헤더 이름은 대소문자를 구분하지 않으며 헤더 슬라이스는 반복 값과 쉼표 목록을 읽습니다. 사용자 TextUnmarshaler는 요청 중에만 실행합니다. map, 일반 중첩 객체, 익명 포인터 임베딩과 중복 이름은 지원하지 않습니다. 공개 익명 값 구조체의 필드는 펼쳐서 바인딩합니다.

기존 `spine.WithValidator(...)` 연결을 유지하세요. 변환 실패 시 validator를 호출하지 않습니다. 검증기는 쿼리와 헤더 구조체를 각각 검증하며 사용자 규칙도 기존 계약을 따릅니다. Playground 개발 변경은 기본 공개 이름 태그를 json, form, query, header 순서로 확장합니다. 직접 WithFieldTags를 지정했다면 필요한 query, header 태그도 포함하세요.

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

## 전환 확인하기

새 CLI는 구버전 Values 인자를 `HTTP_INPUT_API_REMOVED`로 거부하고 구조체 전환을 안내합니다. 다음 명령을 앱 프로젝트에서 실행하세요.

```sh
spine-cli contract check --json
spine-cli verify --json
go test ./...
```

기존 CLI가 새 구조체 입력을 올바르게 판별한다고 가정하지 마세요. 코어와 CLI는 이 변경을 포함한 개발 버전을 함께 검증해야 하며, 생성 템플릿의 공개 버전 고정은 이번 변경으로 자동 갱신하지 않습니다.

명세의 DTO 상세 입력은 등록한 validator가 설명 계약까지 지원할 때 제공하며, 없으면 `INPUT_SCHEMA_DESCRIPTION_UNAVAILABLE`로 이유를 남깁니다. `--source runtime`은 실제 선택한 구현체의 설명을 사용할 수 있습니다. 정적 내보내기는 새 입력의 출처를 식별하지만 아직 필드와 검증 설정 전체를 확장하지 않습니다. 정적 분석의 미해결 항목을 확인하세요.

프로젝트마다 실제 HTTP 테스트도 추가하세요. validator를 연결했다면 누락된 필수 쿼리와 헤더는 400이어야 합니다. 잘못된 숫자는 validator 등록 여부와 무관하게 400이어야 합니다. 요청 본문에 같은 이름의 값을 넣어도 쿼리와 헤더 값이 유지되는지 확인하세요.
