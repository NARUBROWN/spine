# 컨테이너 최소 계약과 역할별 생성자 등록

객체의 역할을 명시해서 생성 계약과 실행 계약을 앱 시작 전에 검사합니다. 모든 역할은 하나의 DI 컨테이너를 공유합니다. 이 문서는 미공개 `v0.7.0-preview.5` 후보 소스의 동작을 설명하며 공개 태그 설치 검증을 뜻하지 않습니다.

## 공통 생성 계약

사용자 정의 컨테이너, Repository, Service, Controller, Interceptor의 객체 등록은 다음 형식을 따릅니다.

```go
func (*UserRepository) Constructor(db *bun.DB) *UserRepository {
    return &UserRepository{db: db}
}
```

- 등록 값은 구조체 포인터이며 `Constructor` 메서드가 있어야 합니다.
- 의존성 인자는 없거나 여러 개일 수 있고, 각 인자의 타입으로 DI에서 해결합니다.
- 반환 값은 정확히 자기 타입 포인터 하나입니다. 다른 타입, 인터페이스, `(T, error)`와 가변 인자는 거부합니다.
- 등록한 객체는 타입을 나타내는 표식입니다. 필드 값은 복사하지 않습니다. 실제 호출 때 새 receiver를 만들고, 반환한 객체를 앱별 singleton으로 관리합니다.
- 메서드는 새 객체를 반환하거나 초기화한 receiver를 반환할 수 있습니다. nil 반환과 패닉은 생성 실패 오류로 처리합니다.

의존성 인자 목록은 구현마다 다르므로 하나의 Go 인터페이스 메서드로 이 형식을 표현하지 않습니다. 예를 들어 `Constructor(...any) any`는 `Constructor(*bun.DB) *UserRepository`와 다른 시그니처입니다. Spine은 공통 생성 계약을 메서드 형식으로 검사하고, 고정된 실행 계약은 Go 인터페이스로 검사합니다.

## 역할별 추가 계약

| 등록 API | 최소 계약 |
| --- | --- |
| `Constructor(&Custom{})` | 공통 생성 계약 |
| `RepositoryConstructor(&UserRepository{})` | 공통 생성 계약 |
| `ServiceConstructor(&UserService{})` | 공통 생성 계약 |
| `ControllerConstructor(&UserController{})` | 공통 생성 계약과 `api.Controller` |
| `InterceptorConstructor(&TxInterceptor{})` | 공통 생성 계약과 `core.Interceptor` |

`api.Controller`는 `RegisterRoutes(*api.Registry)`를 요구합니다. `core.Interceptor`는 기존 `PreHandle`, `PostHandle`, `BeforeResponse`, `AfterCompletion` 계약을 요구합니다. 역할 검사만을 위한 빈 인터페이스나 마커 메서드는 추가하지 않습니다. Repository의 CRUD 메서드와 Service의 업무 메서드는 사용자가 정의합니다.

역할별 API에는 객체 표식을 전달합니다. `&Type{}`, `new(Type)`과 타입이 있는 nil 포인터를 지원합니다. 함수 값을 이 API에 전달하면 생성 계약 오류가 납니다.

## 등록 예제

```go
package example

import (
    "github.com/NARUBROWN/spine"
    "github.com/NARUBROWN/spine/pkg/api"
    "github.com/NARUBROWN/spine/pkg/httpx"
)

type Settings struct { Name string }
func (*Settings) Constructor() *Settings { return &Settings{Name: "Spine"} }

type UserRepository struct { settings *Settings }
func (*UserRepository) Constructor(s *Settings) *UserRepository {
    return &UserRepository{settings: s}
}

type UserService struct { repo *UserRepository }
func (*UserService) Constructor(r *UserRepository) *UserService {
    return &UserService{repo: r}
}

type UserController struct { service *UserService }
func (*UserController) Constructor(s *UserService) *UserController {
    return &UserController{service: s}
}
func (c *UserController) RegisterRoutes(r *api.Registry) {
    r.GET("/name", func() httpx.Response[string] {
        return httpx.Response[string]{Body: c.service.repo.settings.Name}
    })
}

// Build는 등록만 수행합니다. HTTP 플랫폼 선택과 실행은 앱 구성에서 추가합니다.
func Build() spine.App {
    app := spine.New()
    app.Constructor(&Settings{})
    app.RepositoryConstructor(&UserRepository{})
    app.ServiceConstructor(&UserService{})
    app.ControllerConstructor(&UserController{})
    return app
}
```

`var _ api.Controller = ...`는 선택적 컴파일 검사입니다. 역할 등록의 필수 조건은 아니며, 이를 생략해도 Spine이 Controller의 두 계약을 사전 검사합니다.

## 검증 시점과 오류

등록 API는 생성자를 실행하지 않고 오류 정보를 수집합니다. `Validate`, `Prepare`, `Run`이 오류를 반환하며, 잘못된 계약 때문에 생성자나 transport 초기화가 실행되지 않습니다.

| 오류 코드 | 원인 |
| --- | --- |
| `COMPONENT_CONSTRUCTOR_INVALID` | 객체 형식, 생성 메서드 누락, 반환 타입이나 시그니처 오류 |
| `COMPONENT_CONTRACT_INVALID` | Controller 또는 Interceptor의 실행 계약 누락 |
| `COMPONENT_DEPENDENCIES_INVALID` | 중복 제공 타입, 의존성 누락, 인터페이스 제공자 모호성, 순환 의존성 |

객체 등록을 사용하는 앱은 전체 생성자 그래프를 검사합니다. 생성자 본문의 업무 로직, 반환 nil과 패닉은 실제 생성할 때만 확인할 수 있습니다. 명세 조회나 정적 검사 때문에 생성자를 실행하지 않습니다. DI 객체에 임의로 `Close`를 호출하지 않습니다.

## 인터셉터 적용과 호환성

`InterceptorConstructor`는 DI 생성자 등록입니다. 전역 적용, 전송별 범위, 경로별 순서는 기존 `Interceptor`, `InterceptorFor`와 `route.WithInterceptors`로 정합니다. 생성자 등록만으로 인터셉터를 실행하지 않습니다.

기존 `Constructor(NewUserRepository)` 함수 등록도 유지합니다. 외부 라이브러리 타입을 제공하는 기존 생성자나 이전 앱을 그대로 사용할 수 있습니다. 역할별 최소 계약은 새 객체 등록 경로에 적용합니다. 기존 함수 등록만 사용하는 앱은 이전 초기화 정책을 유지합니다.

Controller 역할로 등록한 객체만 HTTP 경로를 수집합니다. Service 등 다른 역할이 우연히 `RegisterRoutes`를 가져도 컨트롤러로 수집하지 않습니다. 기존 범용 함수 등록의 컨트롤러 자동 발견은 호환을 위해 유지합니다.

`spine.App`를 직접 구현한 대체 앱은 네 역할별 등록 메서드도 구현해야 합니다. CLI 후보는 객체 등록의 역할과 생성 메서드 의존성을 정적으로 읽고 계약 위반을 진단하며, 신규 코어 대상 생성 결과도 이 형식을 사용합니다. 정적 분석에서 해석하지 못하는 동적 객체 식은 분석 불완전 오류입니다.

[컨트롤러 경로와 명세](controller-routes.md)에서 HTTP 수집, 입력, 준비된 명세 조회와 전환 절차를 확인하세요.
