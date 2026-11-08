# 컨트롤러 안에서 HTTP 경로 등록하기

컨트롤러의 `RegisterRoutes` 안에 경로와 콜백을 함께 작성합니다. DI 생성자만 앱에 등록하면 되며 별도 `route.go` 연결표는 필요 없습니다.

## 적용 대상

이 문서는 현재 소스의 Preview 후보 API를 설명합니다. 스칼라 경로 입력은 `v0.7.0-preview.8` 후보에서 추가됩니다. 이 문서가 공개 태그나 배포 완료를 뜻하지는 않습니다. 설치할 때는 해당 기능이 포함된 공개 코어와 CLI 태그가 있는지 확인하세요. 이전 코어를 대상으로 하는 CLI 생성 결과는 기존 `app.Route` 방식을 유지합니다.

코어의 Go 하한은 1.25.5입니다. playground 검증기를 사용하는 앱은 해당 모듈의 Go 1.26 이상 조건을 따릅니다.

## 컨트롤러 작성

```go
package controller

import (
    "github.com/NARUBROWN/spine/pkg/api"
    "github.com/NARUBROWN/spine/pkg/httpx"
    "github.com/NARUBROWN/spine/pkg/input"
)

type UserController struct{}
type CreateUser struct { Name string `json:"name"` }
type User struct { Name string `json:"name"` }

func (*UserController) Constructor() *UserController { return &UserController{} }


func (c *UserController) RegisterRoutes(r *api.Registry) {
    r.POST("/users", func(a input.Body[CreateUser]) httpx.Response[User] {
        return httpx.Response[User]{Body: User{Name: a.Name}}
    })
}
```

애플리케이션 구성에서는 컨트롤러 객체의 타입 표식을 등록합니다.

```go
app.Constructor(constructor.Controller(&controller.UserController{}))
```

`constructor`는 `github.com/NARUBROWN/spine/pkg/constructor` 패키지입니다. `constructor.Controller`는 [공통 생성 계약](component-contracts.md)과 `api.Controller`의 `RegisterRoutes(*api.Registry)`를 함께 검사합니다. 생성 메서드는 구체 자기 타입 포인터를 반환합니다. 어느 계약이든 누락되면 생성자 실행 전에 검증 오류가 납니다. Go 인터페이스 `api.Controller` 자체에는 등록 메서드만 있으며, 의존성 인자가 달라지는 생성 계약은 별도로 형식 검사합니다.

추가 메서드의 이름과 공개 여부는 자유입니다. 콜백에서 `c.normalize(a.Name)`을 호출하거나, `RegisterRoutes`가 `c.registerUsers(r)`에 등록을 위임할 수 있습니다. 구조체에 별도 마커를 임베딩할 필요는 없습니다.

## 입력 출처와 필드 이름

| 인자 | 입력 출처 | 기본 필드 이름 |
| --- | --- | --- |
| `input.Path[T]` | URL 경로 매개변수 | `path` 태그가 우선이며 없으면 lowerCamel |
| `input.Query[T]` | URL 쿼리 | `query` 태그가 우선이며 없으면 lowerCamel |
| `input.Body[T]` | 기존 JSON, 폼 등 요청 본문 디코더 | 기존 `encoding/json`과 폼 규칙 유지 |

`input.Query[T]`와 `input.Body[T]`의 `T`는 구조체여야 합니다. `input.Path[T]`는 구조체 또는 기본 스칼라 타입을 허용하며 준비 단계에서 검사합니다. `Name`, `ID`, `UserID`, `HTTPCode`의 기본 경로와 쿼리 이름은 각각 `name`, `id`, `userId`, `httpCode`입니다. 해당 출처 태그의 `-`는 필드를 제외합니다. `json` 태그가 경로나 쿼리 이름을 덮어쓰지는 않습니다.

```go
type UserPath struct { ID int }
type SearchUsers struct {
    UserID int
    Limit int `query:"page_size"`
    Internal string `query:"-"`
}
```

요청 처리에서는 `p.ID`, `q.UserID`, `a.Name`으로 바로 접근합니다. 입력 표식은 이름이 있는 포인터 타입입니다. DTO 메서드가 필요하면 `(*a).Validate()`처럼 명시적으로 호출합니다. 프레임워크가 수동 DTO 메서드를 자동으로 호출하지 않습니다.

한 콜백에 본문 DTO는 하나만 둡니다. 서로 다른 출처를 한 DTO에 섞거나 외부 필드 이름을 중복 선언하면 준비 단계에서 오류가 납니다. 기존 포인터 DTO와 컨텍스트 주입도 계속 사용할 수 있습니다. 바깥쪽 입력 포인터만으로 HTTP 본문의 optional 또는 nullable 여부를 결정하지 않습니다.

선택한 검증기는 실제 `T` 또는 `*T`를 받습니다. 새 필드 이름을 오류에도 사용하려는 검증 구현은 `contract/validator`의 요청별 `FieldNameMapper`를 반영해야 합니다. 함께 변경한 playground 후보는 이 매핑을 사용하며 공유 검증기 설정은 변경하지 않습니다. 이전 구현의 검증 규칙은 계속 동작하지만 태그 없는 필드 오류 이름은 해당 구현의 기존 규칙을 따를 수 있습니다.

### 단일 경로 값

경로 변수가 정확히 하나라면 DTO 없이 기본 스칼라를 사용할 수 있습니다.

```go
r.GET("/users/:id", func(id input.Path[int64]) httpx.Response[int64] {
    return httpx.Response[int64]{Body: *id}
})
```

`input.Path[T]`는 이름이 있는 포인터 타입이므로 스칼라 값은 `*id`로 읽습니다. 문자열, bool, 정수와 부동소수점 기본 타입을 지원합니다. 별도 정의한 타입은 지원하지 않으며 타입 별칭은 기본 타입으로 취급합니다. 경로 변수가 없거나 둘 이상이면 준비 단계에서 거부합니다. 같은 핸들러에 스칼라 경로 인자를 여러 개 선언하거나 같은 키를 다른 DTO로 중복 선언할 수 없습니다. 잘못된 값과 정수 범위 초과는 HTTP 400입니다.

상세 런타임 OpenAPI는 경로 이름과 타입 및 정수의 실제 범위를 표시합니다. 검증기가 없으면 기존 PathsOnly 정책을 따릅니다. 스칼라 인자에는 `validate` 태그를 붙일 수 없으므로 `gt=0` 같은 규칙을 자동 검증하거나 명세에 추가하지 않습니다. 필드별 검증이 필요하면 구조체 DTO를 사용하세요.

현재 발행된 CLI Preview의 정적 계약 검사는 아직 스칼라 `input.Path[T]`를 지원하지 않으며 `HTTP_INPUT_CONTRACT_INVALID`를 반환합니다. CLI 지원이 추가되기 전에는 정적 검사와 함께 사용해야 하는 경로에 구조체 DTO를 사용하세요.

## 준비와 호환성

생성자 함수를 등록하고 별도 `route.go`에서 `app.Route(...)`로 메서드 표현식을 연결하는 기존 컨트롤러 방식은 Deprecated입니다. 새 컨트롤러는 `Constructor` 메서드와 `RegisterRoutes(*api.Registry)`를 정의하고 `constructor.Controller`로 앱에 등록하세요.

전환 기간에는 기존 등록과 새 등록을 한 앱에서 함께 사용할 수 있습니다. 같은 HTTP 메서드와 경로를 두 방식으로 등록하면 준비 단계에서 중복으로 거부합니다. 기존 방식의 제거 버전은 아직 정하지 않았습니다.

`Prepare`는 HTTP가 켜진 앱에서 DI의 구체 반환 타입이 `api.Controller`를 구현하는지 확인하고 실제 인스턴스를 호출합니다. 컨트롤러마다 준비한 앱 인스턴스별로 한 번 수집한 뒤 등록을 고정합니다. 요청이나 문서 조회에서는 다시 실행하지 않습니다. HTTP를 끈 앱은 HTTP 컨트롤러를 생성하지 않습니다.

`GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `HEAD`, `OPTIONS`가 제공됩니다. 기존 `route.WithInterceptors(...)`를 선택 인자로 전달할 수 있습니다. 사용자 구성 오류는 `r.Error(err)`로 전달합니다. 등록 패닉, nil 핸들러, 지원하지 않는 입력이나 반환, 중복 경로와 OpenAPI 리소스 충돌은 리스너 시작 전에 실패합니다.

기존 `app.Route`, 메서드형 HTTP, WebSocket과 이벤트 핸들러는 유지합니다. 사용자 인터셉터가 호출 정보를 읽는 경우 `HandlerMeta.FunctionType()`, `ArgumentOffset()`, `HandlerName()`을 사용하면 메서드와 콜백을 함께 처리할 수 있습니다. `Method` 필드는 기존 메서드 표현식 경로에 남아 있으며 새 콜백의 메서드 정보를 나타내지는 않습니다. 일반 DTO 직접 반환을 새로 지원하지는 않습니다. `httpx.Response[T]`, `error`와 기존 반환 처리기가 지원하는 타입을 사용하세요. 준비 실패 때 임의의 DI 객체나 DB에 `Close`를 자동 호출하는 동작은 추가하지 않습니다.

## 명세 조회와 CLI 검증

준비 전 `spine.DescribeHTTP`는 새 컨트롤러를 발견하면 `spine.ErrRoutesNotCollected`를 반환합니다. 문서 조회 때문에 생성자나 DB 연결을 실행하지 않습니다. 기존 `app.Route`만 있는 앱은 이전 조회 방식을 유지합니다.

준비가 필요한 앱은 호출자가 준비 과정을 명시적으로 수행하고 그 결과를 조회합니다. 준비 과정에는 설정한 DB나 브로커 초기화가 포함될 수 있습니다.

```go
prepared, err := app.Prepare(ctx, options)
if err != nil { return err }
defer prepared.Close(ctx)
description, err := spine.DescribePreparedHTTP(prepared)
```

준비된 명세는 고정된 등록과 입력 계획에서 생성한 독립 복사본입니다. OpenAPI가 켜져 있으면 준비 중 생성하고, 꺼져 있으면 첫 명세 조회 때 한 번 생성합니다. 검증기를 선택하지 않은 경우의 PathsOnly 정책과 `SchemaDescriber`의 상세 제약 정책은 유지합니다. 수동 검증 메서드, 동적 HTTP 상태와 업무 오류는 추측하지 않습니다.

CLI 정적 분석은 `go/types`로 인터페이스 구현과 DI 반환 타입을 판별하고 상수 경로, 타입 별칭, Registry를 전달하는 정적 보조 호출을 읽습니다. 생성자와 `init` 함수를 실행하지 않습니다. 동적 경로나 함수 값, 재귀 등록, 해석하지 못한 조건은 분석 불완전으로 진단합니다.

```sh
spine contract check --json
spine contract export --format openapi --source static --output openapi.json
spine verify --json
```

CLI `--source runtime`도 새 컨트롤러를 위해 `Prepare`를 자동 실행하지 않습니다. 수집 전이면 오류를 반환하므로 정적 내보내기 또는 이미 준비된 앱의 조회를 사용하세요. 새 콜백의 operationId는 런타임과 같은 HTTP 메서드와 최종 경로의 해시를 사용합니다.

## 기존 컨트롤러 전환

1. `NewUserController(...) *UserController` 같은 생성자 함수 대신 `func (*UserController) Constructor(...) *UserController` 메서드를 정의합니다.
2. `app.Constructor(...)`에서 기존 컨트롤러 생성자 등록을 `constructor.Controller(&controller.UserController{})`로 교체합니다.
3. `app.Route(...)`로 연결했던 경로를 `RegisterRoutes`로 옮기고, 옮긴 경로의 기존 등록을 제거합니다. 처리 로직은 기존 메서드에 두고 콜백에서 호출할 수 있습니다.
4. `spine contract check --json`을 실행하고 실제 요청과 명세를 확인합니다.

[구조체 입력 전환](structured-inputs.md)과 [HTTP v3 전환](http-v3.md)을 함께 참고하세요.

## 그룹 경로와 API 문서

현재 개발 소스에서 그룹은 경로 접두사와 문서 태그를 묶습니다. 이 기능이 기존 공개 태그에 포함되어 있다고 가정하지 마세요.

```go
type UserQuery struct {
    ID int64 `query:"id" validate:"gt=0" example:"1" description:"사용자 ID"`
}

func (c *UserController) RegisterRoutes(r *api.Registry) {
    users := r.Group("/users", api.Tags("Users"))
    users.GET(
        "/",
        func(q input.Query[UserQuery]) httpx.Response[User] {
            return httpx.Response[User]{Body: User{Name: "example"}}
        },
        api.Summary("사용자 조회"),
        api.Description("사용자 ID로 조회합니다."),
    )
}
```

`users.GET("/", handler, options...)`은 `GET /users`를 등록합니다. 하위 그룹은 접두사와 태그를 상속하며 중복 태그는 제거합니다. 옵션이 없으면 핸들러 뒤의 인자를 생략합니다. `api.Interceptors(...)`는 그룹과 라우트에 적용할 인터셉터를 추가합니다. `Registry.GET`과 `Group.GET`은 모두 `path, handler, options...` 순서를 사용합니다. 이 통일된 순서는 `v0.7.0-preview.10` 이상을 대상으로 하며, `v0.7.0-preview.9`까지는 그룹 라우트에 `api.Options(...), handler` 순서를 사용합니다.

`example`과 `description` 필드 태그는 문서용이며 런타임 기본값이나 검증 규칙이 아닙니다. 지원하지 않는 타입이나 잘못된 예시는 명세 진단에 표시됩니다. 숫자 범위, 문자열 길이, 열거값과 필수값을 포함해 명세로 표현된 제약을 예시와 대조합니다. 이메일, URI, UUID와 정규식처럼 명세만으로 실제 검증기와 같은 결과를 보장할 수 없는 경우에는 `FIELD_EXAMPLE_VALIDATION_UNRESOLVED` 진단을 남깁니다. 사용자 정의 검증기는 실행하지 않습니다. 자동 생성 명세의 정수 경계는 그대로 유지하며 내장 UI에서는 타입 자체의 범위만 간결하게 표시합니다. JavaScript 클라이언트의 안전 정수 범위를 넘는 숫자는 별도 처리가 필요합니다.

CLI 정적 분석은 상수 그룹 경로와 `api.Tags`, `api.Summary`, `api.Description`, 식별 가능한 `api.Interceptors`를 읽습니다. 정적으로 확인되는 보조 함수에 그룹을 전달해도 경로와 옵션을 추적하며, 인터셉터의 선언 위치는 `interceptor_sources`에 보존합니다. 동적 그룹이나 인터셉터 값, 확장된 인터셉터 목록은 미해결 진단을 반환합니다. 정적 분석은 사용자 코드를 실행하지 않습니다.

CLI가 정적으로 내보낸 OpenAPI는 검증기를 실행하지 않으므로 쿼리 필드의 필수 여부와 검증 제약을 확정하지 않습니다. 해당 매개변수에는 `x-spine-validation-unresolved`가 표시됩니다. 실제 검증기를 반영한 명세는 준비된 앱의 `/openapi.json`에서 확인하세요.
