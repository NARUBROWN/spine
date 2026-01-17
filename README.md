# 🦴 Spine
<p align="center">
  <img src="assets/spine-logo.png" alt="Spine" width="420" />
</p>

**Spine is the execution backbone of a backend system.**

Spine defines **how a request is resolved, executed, and completed** — explicitly.

**Spine는 요청 실행(Runtime)을 1급 개념으로 다루는 백엔드 웹 프레임워크입니다.**

---

## Spine의 문제의식

대부분의 웹 프레임워크는 실행 흐름을 숨깁니다.

- 요청이 어디서 시작되는지
- 누가 인자를 생성하는지
- 언제 비즈니스 코드가 실행되는지
- 반환값이 어떻게 응답으로 변환되는지

이 모든 과정은 프레임워크 내부 규칙과 관습, 암묵적 동작에 묻혀 있습니다.

Spine는 이 흐름을 숨기지 않습니다.  
**실행 순서와 책임을 코드 구조로 고정**합니다.

---

## 한국과 Spine

한국에는 자체 IoC + 실행 파이프라인 구조를 가진 백엔드 프레임워크가 거의 없습니다.  
전자정부 프레임워크조차 Spring IoC 위의 조합물에 가깝습니다.

NestJS, Spring, FastAPI, Django는 모두 해외 설계 철학의 수입입니다.

Spine는 한국에서 거의 처음 시도되는,  
**Execution Pipeline 중심의 현대적인 백엔드 프레임워크**입니다.

---

## Spine으로 만든 예제 프로젝트

👉 https://github.com/NARUBROWN/spine-user-demo

---

## Spine은 무엇이 아닌가

- ❌ HTTP Engine이 아닙니다
- ❌ Router 중심 프레임워크가 아닙니다
- ❌ Annotation / Decorator 기반 프레임워크가 아닙니다
- ❌ Convention over Configuration을 채택하지 않습니다
- ❌ Controller에 실행 책임을 위임하지 않습니다

Spine는 **Execution Pipeline**입니다.

---

## 전체 아키텍처 개요

```
HTTP Transport (Echo)
        │
        ▼
ExecutionContext
        │
        ▼
Pipeline
  ├─ Router
  ├─ ParameterMeta Builder
  ├─ ArgumentResolver Chain
  ├─ Interceptor (preHandle)
  ├─ Invoker (Controller Method Call)
  ├─ ReturnValueHandler
  └─ Interceptor (postHandle)
        │
        ▼
ResponseWriter
```

이 구조는 설정이 아니라 **실행 모델 그 자체**입니다.

---

## Execution Pipeline

모든 요청은 다음 순서를 **반드시** 따릅니다.

1. Pipeline 진입
2. Router를 통한 HandlerMeta 선택
3. ParameterMeta 생성
4. ArgumentResolver 체인 실행
5. Interceptor.preHandle
6. Controller Method 호출 (Invoker)
7. ReturnValueHandler 실행
8. Interceptor.postHandle
9. ResponseWriter를 통한 응답 기록

이 순서는 숨겨지지 않으며, 암묵적으로 변경되지 않습니다.

---

## Controller 설계 철학

### Spine의 원칙

> **Controller는 실행 모델을 모른다.  
> 하지만 입력의 출처는 타입으로 명시한다.**

Controller는 다음에 **의존할 수 있습니다**:

- `path.*` : Path Parameter 의미 타입
- `query.*` : Query Parameter 의미 타입
- `httperr.*` : HTTP Error 의미 타입

Controller는 다음에 **의존하지 않습니다**:

- ExecutionContext
- Pipeline
- Router
- Resolver
- HTTP / Transport 타입

---

### Controller 예시

```go
func (c *UserController) GetUser(userId path.Int) (User, error) {
    if userId.Value <= 0 {
        return User{}, httperr.BadRequest("유효하지 않은 사용자 ID입니다")
    }

    user, err := c.repo.FindByID(userId.Value)
    if err != nil {
        return User{}, httperr.NotFound("사용자를 찾을 수 없습니다")
    }

    return user, nil
}
```

Controller는:
- HTTP를 모릅니다
- 실행 순서를 모릅니다
- 값의 출처만 명시합니다

---

## Signature-as-Contract

Spine에서 **메서드 시그니처는 API 계약 그 자체**입니다.

- 입력 생성 → `ArgumentResolver`
- 출력 처리 → `ReturnValueHandler`

시그니처 변경은 곧 API 변경입니다.

Spine는 다음을 의도적으로 금지합니다:

- ❌ Annotation 기반 매핑
- ❌ Convention 기반 자동 추론
- ❌ Primitive 타입의 암묵적 주입

---

## Context 분리

Spine는 Context를 두 계층으로 분리합니다.

### ExecutionContext

- 실행 흐름 제어 전용
- Router / Pipeline 내부에서만 사용
- Controller 및 Resolver에 노출 ❌

### RequestContext

- 입력 해석 전용 (Path / Query / Body)
- ArgumentResolver에서만 사용
- Controller에 노출 ❌

---

## Path Parameter Binding Rule

Spine의 Path Parameter 바인딩은 **순서 기반(order-based)** 입니다.  
이는 Go 언어 제약을 고려한 **의도적이고 명시적인 계약**입니다.

### 규칙

```
Route Path Key 선언 순서
=
Controller 시그니처의 path.* 파라미터 선언 순서
```

### 예시

```go
// Route
/users/:userId/posts/:postId

// Controller
func GetPost(userId path.Int, postId path.Int)
```

### 정책

- 이름 매칭 ❌
- annotation ❌
- primitive 타입 ❌
- 순서 불일치 시 즉시 실패 (Fail Fast)

---

## Query 처리 원칙

### 의미가 고정된 Query

```go
func ListUsers(p query.Pagination)
```

Spine가 제공하는 의미 타입을 사용합니다.

### 가변 Query

```go
func SearchUsers(q query.Values)
```

- Raw Query View 제공
- 사용자가 직접 해석
- DTO 자동 매핑 ❌

---

## ReturnValueHandler & ResponseWriter

Controller는 값을 반환합니다.

```go
return User{...}
```

응답 생성은 전부 `ReturnValueHandler`가 담당합니다.  
Transport는 `ResponseWriter`만 구현합니다.

---

## License

MIT
