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

## 지원하는 필드와 검증기

태그가 있는 공개 필드만 바인딩합니다. 스칼라, 스칼라 포인터와 슬라이스, RFC3339의 time.Time, encoding.TextUnmarshaler를 지원합니다. 쿼리 슬라이스는 반복 키에서 읽습니다. 헤더 이름은 대소문자를 구분하지 않으며 헤더 슬라이스는 반복 값과 쉼표 목록을 읽습니다. 사용자 TextUnmarshaler는 요청 중에만 실행합니다. map, 일반 중첩 객체, 익명 포인터 임베딩과 중복 이름은 지원하지 않습니다. 공개 익명 값 구조체의 필드는 펼쳐서 바인딩합니다.

기존 `spine.WithValidator(...)` 연결을 유지하세요. 변환 실패 시 validator를 호출하지 않습니다. 검증기는 쿼리와 헤더 구조체를 각각 검증하며 사용자 규칙도 기존 계약을 따릅니다. Playground 개발 변경은 기본 공개 이름 태그를 json, form, query, header 순서로 확장합니다. 직접 WithFieldTags를 지정했다면 필요한 query, header 태그도 포함하세요.

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
