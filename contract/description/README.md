# 런타임 OpenAPI

HTTP 앱은 `Run`, `RunContext`, `Prepare`에서 기본적으로 `/openapi.json`을 제공합니다. 실제 등록한 라우트와 Go 타입으로 OpenAPI 3.1.2 JSON을 생성합니다. CLI, 소스 파일, 문서 주석과 swaggo가 필요하지 않습니다. 문서 생성과 요청 처리는 같은 기본 리졸버 목록, 반환 처리 목록과 경로 인자 배정 규칙을 사용합니다.

## 기본 동작과 설정

기존 `app.Route`, DTO와 `httpx.Response[T]` 사용을 유지합니다. `HTTP: nil`이면 HTTP와 명세 리소스가 모두 비활성화됩니다. 파일은 자동으로 쓰지 않습니다.

```go
options := boot.Options{
    Address: ":8080",
    HTTP: &boot.HTTPOptions{
        GlobalPrefix: "/api",
        OpenAPI: boot.OpenAPIOptions{
            Path:    "/openapi.json",
            Title:   "User API",
            Version: "1.0.0",
        },
    },
}
```

위 코드는 `github.com/NARUBROWN/spine/pkg/boot`를 import한 함수 내부의 옵션 예입니다. 이 옵션으로 앱을 실행하면 명세의 API 경로에는 `/api`가 반영되고, 명세 자체는 prefix와 독립된 `/openapi.json`에서 제공됩니다.

| 옵션 | 기본값 | 동작 |
| --- | --- | --- |
| `Disabled` | `false` | `true`이면 명세 리소스를 제공하지 않음 |
| `Path` | `/openapi.json` | GET·HEAD를 제공하는 정적 절대 경로 |
| `Title` | `Spine API` | OpenAPI `info.title` |
| `Version` | `unversioned` | API 문서 버전. OpenAPI 규격 버전과 별개 |

같은 경로의 애플리케이션 GET·HEAD 또는 WebSocket 등록과 충돌하면 사전 검사에서 `OPENAPI_PATH_CONFLICT`로 거부합니다. HTTP 플랫폼에 직접 등록하는 native 경로는 코어가 검사할 수 없으므로 다른 경로를 선택해야 합니다.

명세는 준비 전에 생성자나 외부 자원을 초기화하지 않고 생성한 뒤, 준비 단위로 고정합니다. 준비가 끝난 뒤 원본 앱의 등록이나 설명 결과를 수정해도 이미 실행 중인 명세는 바뀌지 않습니다.

## 문서 요청의 실행 정책

명세 요청은 HTTP 플랫폼의 서버 미들웨어를 통과하지만 비즈니스 컨트롤러, DTO 바인딩, validator와 Spine 인터셉터 파이프라인은 실행하지 않습니다. 문서를 조회하면서 트랜잭션을 열거나 이벤트를 발행하지 않습니다. 문서는 기본적으로 조회 가능하며, 접근 제한이 필요하면 서버 미들웨어를 적용하거나 `Disabled`를 설정합니다.

## 앱을 실행하지 않고 설명하거나 내보내기

```go
description, err := spine.DescribeHTTP(app, options)
if err != nil {
    return err
}
document, err := openapi.Generate(description, options.HTTP.OpenAPI)
if err != nil {
    return err
}
_, err = output.Write(document)
return err
```

위 코드는 `spine`과 `github.com/NARUBROWN/spine/pkg/openapi`를 import한 함수 내부 예입니다. `app`, HTTP가 활성화된 `options`, `io.Writer`인 `output`을 받습니다. `DescribeHTTP`는 생성자, factory, 핸들러나 검증 규칙을 실행하지 않습니다. 검증기가 `SchemaDescriber`를 구현하면 부작용 없는 규칙 설명 메서드만 호출합니다.

기존 `spine.App` 인터페이스에는 메서드를 추가하지 않았습니다. 표준 앱은 선택적 `spine.HTTPDescriber`를 구현합니다. 이 기능이 없는 사용자 정의 App에는 `ErrHTTPDescriptionUnavailable`을 반환합니다.

## 검증 규칙

등록된 validator만 검증 제약을 설명할 수 있습니다. DTO에 `validate` 태그가 있어도 validator를 등록하지 않았다면 명세 생성기는 그 태그를 검증 규칙으로 해석하지 않습니다. `json:",omitempty"`도 입력 필수값 판정에 사용하지 않습니다.

`contract/validator.SchemaDescriber`는 Go 필드 인덱스와 JSON Schema 2020-12 제약을 연결합니다. 기존 `Validator` 인터페이스는 유지됩니다. 설명 기능이 없는 validator도 정상 동작하며, 명세에는 `VALIDATION_DESCRIPTION_UNAVAILABLE` 진단을 남깁니다. 지원하지 않는 사용자 규칙, 태그 이름 변경과 규칙 덮어쓰기는 실제 validator 구현이 판단해야 합니다. 명세를 얻으려고 `Validate`에 빈 DTO를 넣지 않습니다.

검증기의 숫자 `required`는 0, bool `required`는 false도 거부할 수 있습니다. 이런 제약은 OpenAPI의 `required` 배열 외에 값 제약으로도 표현해야 합니다. 포인터의 필수값은 null과 필드 누락을 구분해 설명합니다. HTTP 400에는 바인딩 오류의 `message`와 DTO 검증 오류의 선택적 `fields`를 기술합니다.

## 현재 지원 범위

- 등록된 메서드·경로·prefix와 경로 인자, `query.Pagination`의 기본값
- JSON DTO, form 태그가 있는 DTO의 JSON·urlencoded·multipart 입력
- `httpx.Response[T]`의 본문 타입, 문자열 응답, 바이너리와 redirect 구분
- JSON 이름, 익명 임베딩의 우선순위·충돌, `omitempty`·`omitzero`, 포인터·slice·map의 null
- `time.Time`, `json.RawMessage`, byte slice, 중첩·재귀·제네릭 타입
- 등록된 검증기가 설명하는 제약조건

명세에 표현되지 않은 동작은 `x-spine-diagnostics`로 구분합니다. 동적 query/header 키, adapter별 쿼리 DTO 바인딩, 업로드 필드 이름, 사용자 정의 JSON/text 직렬화와 동적 status·Content-Type은 완전히 추론하지 않습니다. `ResponseOptions.Status`나 서비스 내부 오류를 얻으려고 핸들러를 실행하지 않습니다. 응답 상태는 `default`로 남기며 본문은 선언된 반환형과 프레임워크 오류 형식을 함께 설명합니다.

HTTP 플랫폼에 직접 등록한 native 경로, WS 메시지와 event payload는 이번 OpenAPI 대상에 포함하지 않습니다. Swagger UI와 ReDoc은 이 JSON을 읽는 별도 표시 계층으로 추가할 수 있습니다.

## 검증

```sh
go test ./...
bash scripts/check-module-boundaries.sh
go -C test test . -run '^TestOpenAPI' -count=1
```

마지막 검사는 실제 HTTP 진입점과, Go 소스·CLI가 없는 디렉터리에서 실행 파일만 사용하는 경로를 검증합니다. 코어와 공개 계약은 표준 라이브러리만 사용합니다.
