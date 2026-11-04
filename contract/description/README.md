# 런타임 OpenAPI

이 개발 소스의 HTTP 앱은 `Run`, `RunContext`, `Prepare`에서 기본적으로 `/openapi.json`을 제공합니다. 실제 등록한 라우트와 Go 타입으로 OpenAPI 3.1.2 JSON을 생성합니다. CLI, 소스 파일, 문서 주석과 swaggo는 필요하지 않습니다. 문서 생성과 요청 처리는 같은 기본 리졸버 목록, 반환 처리 목록과 경로 인자 배정 규칙을 사용합니다. 정식 공개 여부는 릴리스와 사용하는 코어 버전으로 확인하세요.

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
| `Disabled` | `false` | `true`이면 명세 자동 생성과 HTTP 제공을 끔 |
| `Path` | `/openapi.json` | GET, HEAD를 제공하는 정적 절대 경로 |
| `Title` | `Spine API` | OpenAPI `info.title` |
| `Version` | `unversioned` | API 문서 버전. OpenAPI 규격 버전과 별개 |

명세 경로에 매칭되는 앱 GET, HEAD 또는 WebSocket 경로는 `OPENAPI_PATH_CONFLICT`로 거부합니다. `/:id`처럼 경로 인자가 겹치는 경우도 포함합니다. 플랫폼에 직접 등록한 native 경로는 코어가 검사할 수 없습니다.

명세는 준비 전에 생성자나 외부 자원을 초기화하지 않고 생성한 뒤, 준비 단위로 고정합니다. 준비가 끝난 뒤 원본 앱의 등록이나 설명 결과를 수정해도 이미 실행 중인 명세는 바뀌지 않습니다.

관리 실행은 실제로 바인딩한 포트의 명세 URL을 앱 로그에 출력합니다. `spine run`, `spine dev`에서도 이 로그를 확인할 수 있습니다.

## 문서 요청의 실행 정책

명세는 기본 공개 리소스입니다. 요청은 HTTP 플랫폼의 서버 미들웨어를 통과하지만 비즈니스 컨트롤러, DTO 바인딩, validator와 Spine 인터셉터 파이프라인은 실행하지 않습니다. 문서를 조회하면서 트랜잭션을 열거나 이벤트를 발행하지 않습니다. 부팅 옵션의 `HTTP.OpenAPI.Disabled`를 true로 설정하면 명세 자동 생성, HTTP 제공과 문서 경로 충돌 검사를 끕니다. 앱을 명시적으로 설명하는 DescribeHTTP와 openapi.Generate는 계속 사용할 수 있습니다.

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

같은 핸들러에서 동일한 구조체 타입의 입력 DTO를 중복 선언하면 `HTTP_INPUT_DTO_DUPLICATED`로 거부합니다. 서로 다른 본문 DTO도 하나를 초과하면 `HTTP_BODY_PARAMETERS_DUPLICATED`로 거부하고, 같은 출처의 입력 키를 여러 DTO가 선언하면 `HTTP_INPUT_KEY_DUPLICATED`로 거부합니다. 타입 별칭도 같은 타입으로 판정하며 다른 라우트에서의 재사용은 허용합니다. 앱 사전 검사와 HTTP 설명은 같은 검사 규칙을 사용합니다.

기존 `spine.App` 인터페이스에는 메서드를 추가하지 않았습니다. 표준 앱은 선택적 `spine.HTTPDescriber`를 구현합니다. 이 기능이 없는 사용자 정의 App에는 `ErrHTTPDescriptionUnavailable`을 반환합니다.

## 검증 규칙

등록된 validator만 검증 제약을 설명할 수 있습니다. validator가 없으면 경로와 HTTP 메서드만 설명하고 `VALIDATOR_NOT_CONFIGURED`로 이유를 남깁니다. 입력 파라미터, 요청과 응답 모델, 오류 상태 코드를 추정하지 않습니다. 표준 OpenAPI에 필요한 최소 응답 자리와 타입을 지정하지 않은 경로 변수 자리만 유지합니다. validator는 있지만 설명 계약을 지원하지 않으면 DTO 상세 입력을 생략하고 `INPUT_SCHEMA_DESCRIPTION_UNAVAILABLE`로 이유를 남깁니다. `json:",omitempty"`도 입력 필수값 판정에 사용하지 않습니다.

`contract/validator.SchemaDescriber`는 Go 필드 인덱스와 JSON Schema 2020-12 제약을 연결합니다. 기존 `Validator` 인터페이스는 유지됩니다. 설명 기능이 없는 validator도 요청 검증은 정상 동작하지만 상세 입력 명세는 생략합니다. 지원하지 않는 사용자 규칙, 태그 이름 변경과 규칙 덮어쓰기는 실제 validator 구현이 판단해야 합니다. 명세를 얻으려고 `Validate`에 빈 DTO를 넣지 않습니다.

검증기의 숫자 `required`는 0, bool `required`는 false도 거부할 수 있습니다. 이런 제약은 OpenAPI의 `required` 배열 외에 값 제약으로도 표현해야 합니다. 포인터의 필수값은 null과 필드 누락을 구분해 설명합니다. 경로 이름 불일치, 같은 라우트의 중복 경로 변수와 기존 래퍼 및 Path DTO의 혼용은 validator나 명세 활성화 여부와 무관하게 부팅 전에 거부합니다.

HTTP 400에는 바인딩 오류의 `message`, 출처인 `in`과 DTO 검증 오류의 선택적 `fields`를 기술합니다. 검증 필드에는 입력 출처와 실제 바인딩 필드명을 사용합니다. 태그 없는 본문과 검증 태그 없는 DTO도 사용할 수 있습니다.

## 기존 입력과 반환형 사용

명세 생성은 기존 DTO와 핸들러 반환형을 읽습니다. 명세를 위해 입력을 래퍼로 감싸거나 라우트에 응답 타입을 다시 선언하지 않습니다.

```go
type CreateUserDTO struct {
    Name string `json:"name" validate:"required,min=2"`
}
func (*UserController) Create(input *CreateUserDTO) httpx.Response[UserDTO] {
    return httpx.Response[UserDTO]{Body: UserDTO{Name: input.Name}}
}

app.Route("POST", "/users", (*UserController).Create)
```

위 코드는 앱이 정의한 `UserController`, `UserDTO`와 `pkg/httpx`를 사용하는 예입니다. 입력 필드는 `input.Name`으로 직접 읽습니다. 명세는 등록된 메서드와 경로, JSON 필드, 반환형의 본문 타입을 추출합니다. 검증기를 등록하면 설명 가능한 `validate` 규칙도 반영합니다.

경로, 쿼리와 헤더는 각각 별도 구조체 포인터로 받습니다. path 태그는 라우트의 경로 변수 이름과 정확히 연결하며 구조체 필드 순서에 의존하지 않습니다. 경로 파라미터는 검증 태그와 무관하게 required로 설명하고 포인터 필드도 null로 표시하지 않습니다. `query` 필드 태그가 있는 DTO는 쿼리에서, `header` 필드 태그가 있는 DTO는 헤더에서만 읽습니다. 함수 안에서는 일반 필드로 직접 접근하며 본문이 값을 덮어쓰지 않습니다. 서로 다른 출처의 필드는 한 DTO에 섞지 않습니다. query.Values와 header.Values는 제거했습니다. 기존 path.Int, path.String, path.Boolean은 전환 기간에 단독 사용을 유지하며 새 Path DTO와 혼용하면 거부합니다. [구조체 입력 전환 안내](../../docs/migration/structured-inputs.md)를 참고하세요.

파일은 기존 `form` 태그와 표준 라이브러리의 `*mime/multipart.FileHeader`, `[]*mime/multipart.FileHeader`, `[]mime/multipart.FileHeader`에서 필드 이름과 개수를 추출합니다. 파일 필드가 있는 DTO는 multipart/form-data로만 기술합니다. 현재 Echo 기본 바인더와 대조했으며 다른 플랫폼은 자체 바인더의 지원 여부를 확인해야 합니다. 동적인 `multipart.UploadedFiles`는 필드 이름을 추측하지 않습니다.

`httpx.Response[T]`의 T에서 응답 본문 타입을 추출합니다. 실제 상태 코드는 기존 `ResponseOptions.Status`와 반환 오류가 결정합니다. 명세 생성기는 핸들러를 실행하지 않으므로 상태 코드를 `default`로 남깁니다. 문자열은 text/plain으로 설명하고, `httpx.Binary.ContentType`처럼 동적인 미디어 타입은 진단으로 구분합니다. 런타임 값은 별도 문서용 선언으로 보충하지 않습니다.

## CLI 연동

CLI 연동은 코어 입력 스펙 확정 후 별도 이슈로 진행합니다. 현재 변경의 명세 생성과 입력 검사는 런타임 코어의 동작이며 CLI 변경을 전제로 하지 않습니다.

## 현재 지원 범위

다음 상세 설명 범위는 validator가 설정된 경우에 적용하며 상세 입력에는 SchemaDescriber가 필요합니다.

- 등록된 메서드, 경로, prefix와 경로 인자, `query.Pagination`의 기본값
- JSON DTO, form 태그가 있는 DTO의 JSON, urlencoded, multipart 입력
- `httpx.Response[T]`의 본문 타입, 문자열 응답, 바이너리와 redirect 구분
- JSON 이름, 익명 임베딩의 우선순위, 충돌, `omitempty`, `omitzero`, 포인터, slice, map의 null
- `time.Time`, `json.RawMessage`, byte slice, 중첩, 재귀, 제네릭 타입
- 등록된 검증기가 설명하는 제약조건

같은 메서드의 경로가 충돌하면 기존 앱 준비 단계처럼 거부합니다. 같은 URL 형태에서 메서드별 경로 인자 이름이 다르면 하나의 OpenAPI Path Item으로 묶습니다.

명세에 표현되지 않은 동작은 `x-spine-diagnostics`로 구분합니다. 동적 업로드 필드 이름, 사용자 정의 JSON/text 직렬화와 동적 status, Content-Type은 완전히 추론하지 않습니다. `ResponseOptions.Status`나 서비스 내부 오류를 얻으려고 핸들러를 실행하지 않습니다. 응답 상태는 `default`로 남기며 본문은 선언된 반환형과 프레임워크 오류 형식을 함께 설명합니다.

HTTP 플랫폼에 직접 등록한 native 경로, WS 메시지와 event payload는 이번 OpenAPI 대상에 포함하지 않습니다. Swagger UI와 ReDoc은 이 JSON을 읽는 별도 표시 계층으로 추가할 수 있습니다.

## 검증

```sh
go run ./scripts/test test ./...
bash scripts/check-module-boundaries.sh
go -C test test . -run '^TestOpenAPI' -count=1
go -C test/openapi test -race ./... -count=1
```

test 모듈은 실제 HTTP 진입점과 소스, CLI가 없는 디렉터리에서 실행 파일만 사용하는 경로를 검증합니다. test/openapi는 Go 1.26 이상에서 공식 OpenAPI 구조 스키마, 독립 JSON Schema 검사와 실제 Echo 요청을 대조합니다. 검증한 경계 입력의 일치는 해당 표본의 증거이며 모든 사용자 규칙의 완전한 변환을 뜻하지 않습니다. 코어와 공개 계약은 표준 라이브러리만 사용하며 외부 검증 의존성은 별도 테스트 모듈에 둡니다.
