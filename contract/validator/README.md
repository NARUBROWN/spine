# DTO 검증 공통 계약

`github.com/NARUBROWN/spine/contract/validator`는 외부 검증 구현이 DTO 검증 결과를 공통 오류로 전달하는 공개 패키지입니다. Spine 모듈에 포함되며 표준 라이브러리만 사용합니다. 계약 식별자는 `spine.validation/v1`입니다.

앱은 `spine.WithValidator`로 검증기를 선택합니다. HTTP JSON·폼 DTO와 WebSocket·컨슈머 메시지 DTO의 바인딩 또는 역직렬화가 성공하면 핸들러 호출 전에 검증합니다. 구체 검증 라이브러리는 외부 구현이 제공하며 Spine 코어에는 포함하지 않습니다.

## 앱에 연결

```go
import (
    "github.com/NARUBROWN/spine"
    platform "github.com/NARUBROWN/spine-echo-platform"
    validation "github.com/NARUBROWN/spine/contract/validator"
)

func newApplication(engine validation.Validator) spine.App {
    return spine.New(platform.Option(), spine.WithValidator(engine))
}
```

옵션을 생략하면 기존처럼 DTO 바인딩만 수행합니다. `WithValidator(nil)`이나 타입 정보가 있는 nil을 등록하면 `App.Validate`, `Prepare`, `Run`에서 `ErrInvalidValidator`로 실패합니다. 사전 검사는 `CheckValidator`로 등록 여부만 확인하며 DTO 검증을 실행하지 않습니다. 준비 결과는 같은 검증기 인스턴스를 공유합니다.

HTTP는 JSON, 폼, 별도 쿼리와 헤더 DTO의 포인터를, WebSocket과 컨슈머는 핸들러에 전달할 구조체 값을 검증기에 전달합니다. 바인딩 실패 시 검증기를 호출하지 않으며 DTO가 아닌 컨텍스트, 경로 인자와 query.Pagination 등은 검증하지 않습니다. 기존 인증 인터셉터는 바인딩과 검증보다 먼저 실행합니다.

| 경로 | 규칙 위반 처리 | 검증기 처리 오류 |
| --- | --- | --- |
| HTTP | 400과 `message`, `fields` JSON 응답. 핸들러 실행 중단 | 500의 일반 오류 응답 |
| WebSocket | 1008과 `invalid message`로 연결 종료. 핸들러 실행 중단 | 기존 1011과 `handler error`로 연결 종료 |
| 컨슈머 | 핸들러 실행 중단 후 기존 NACK·재전달 정책 적용 | 기존 실패 처리 정책 적용 |

위 정책은 Spine 런타임이 소유하며 검증 계약 자체에는 HTTP 상태나 브로커 정책을 넣지 않습니다. HTTP의 구체 `httperr.HTTPError`를 함께 반환하면 기존 HTTP 상태 처리가 우선합니다. 응답 전 인터셉터와 트랜잭션 종료 훅에는 원래 검증 오류가 전달됩니다.

## 검증 인터페이스

```go
type Validator interface {
    Validate(ctx context.Context, value any) error
}
```

| 입력 | 계약 |
| --- | --- |
| `ctx` | 호출자가 제공하는 nil이 아닌 context. 취소되면 `ctx.Err()`를 보존한 오류를 반환합니다. |
| `value` | 바인딩이나 역직렬화를 마친 구조체 또는 nil이 아닌 구조체 포인터. 그 외의 값은 `ErrInvalidValue`로 감싼 오류를 반환합니다. |

검증은 동기 실행하며 한 인스턴스를 여러 요청에서 동시에 사용할 수 있어야 합니다. 구현은 입력을 변경하거나 반환 후 보관하지 않습니다. 규칙 등록과 설정은 첫 검증 전에 마칩니다. 필드의 nil 허용 여부, `required`가 0이나 false를 허용하는지, 사용자 정의 규칙 등은 구체 구현이 문서화합니다.

| 반환값 | 의미 |
| --- | --- |
| `nil` | 검증 성공 |
| `*validation.Error` 또는 이를 감싼 오류 | DTO 규칙 위반. `Fields`에는 오류가 하나 이상 있어야 합니다. |
| `ErrInvalidValue`로 감싼 오류 | 지원하지 않는 DTO 타입 또는 nil DTO를 전달한 호출 오류 |
| `ErrInvalidValidator`로 감싼 오류 | nil 검증기 등록 또는 비어 있는 검증 실패를 반환한 계약 위반 |
| 취소·잘못된 설정·그 밖의 오류 | 검증을 정상 수행하지 못함. DTO 규칙 위반으로 분류하지 않습니다. |

계약에는 HTTP 상태 코드, WebSocket 종료 코드나 브로커 재시도 정책을 넣지 않습니다. 호출자가 검증 결과를 각 전송 방식의 처리 정책에 연결합니다. 계약 식별자는 문서와 구현의 호환 기준이며 이 패키지가 실행 시 버전 협상을 수행하지는 않습니다.

## 필드별 오류

`validation.Error`는 다음 필드 목록을 `Fields []validation.FieldError`로 제공합니다.

| 필드 | JSON 키 | 의미 |
| --- | --- | --- |
| `Field` | `field` | 외부 필드 이름을 사용하는 [JSON Pointer](https://www.rfc-editor.org/rfc/rfc6901). 예: `/items/0/email`. 전체 DTO 오류는 빈 문자열입니다. |
| `Rule` | `rule` | 구현이 문서화한 기계 판독용 규칙 이름. 예: `required`, `email`, `gte`. |
| `Param` | `param` | 규칙 설정값. 예: `gte=18`의 `"18"`. 비어 있으면 JSON에서 생략합니다. |
| `Message` | `message` | 표시용 설명. 오류 분류에 사용하지 않으며 비어 있으면 JSON에서 생략합니다. |

객체 키에 `~`나 `/`가 있으면 각각 `~0`, `~1`로 이스케이프합니다. 예를 들어 `settings` 아래의 `a/b~c` 키는 `/settings/a~1b~0c`입니다. JSON DTO는 `json` 이름, 폼 DTO는 `form` 이름 등 구체 구현이 입력 이름을 연결하는 규칙을 문서화해야 합니다.

같은 필드의 여러 규칙 위반과 전체 DTO 오류를 함께 전달할 수 있습니다. 구현은 동일한 입력에 대해 결정적인 오류 순서를 유지합니다. 오류에는 거부된 입력값을 담는 필드를 두지 않으며, `Param`과 `Message`에도 입력값을 자동으로 복사하지 않습니다. 반환된 오류 목록은 읽기 전용으로 사용합니다.

`NewError(fields...)`는 오류가 없으면 nil인 `error`를 반환합니다. 오류가 있으면 목록을 복사하여 순서를 유지하므로 호출자가 원래 슬라이스를 재사용할 수 있습니다. `Error()` 문자열은 오류 개수의 요약이며 세부 분류는 `errors.As`와 `Fields`를 사용합니다.

`AsError(err)`는 감싸진 규칙 위반의 세부 정보를 반환합니다. 취소·구현 오류가 함께 포함되거나 필드 목록이 비어 있으면 규칙 위반으로 분류하지 않습니다. 런타임은 검증 중 발생한 취소를 다시 확인하며 검증기가 반환한 원인도 함께 보존합니다. 비어 있는 `*Error`나 nil인 `*Error`를 error 인터페이스로 반환하면 `ErrInvalidValidator`로 처리합니다.

```go
err := validation.NewError(validation.FieldError{
    Field: "/age",
    Rule:  "gte",
    Param: "18",
})

var failure *validation.Error
if errors.As(err, &failure) {
    // failure.Fields를 응답이나 메시지 처리 정책에 연결합니다.
}
```

위 코드는 `errors`와 `validation "github.com/NARUBROWN/spine/contract/validator"`를 import한 함수 내부의 예입니다. 외부 구현이 공개 계약만으로 동작하는 전체 예는 [contract_test.go](../../test/unit/contract/validator/contract_test.go)의 `ExampleValidator`에서 확인할 수 있습니다.

```sh
go run ./scripts/test test ./contract/validator -count=1
bash scripts/check-module-boundaries.sh
```

[spine-validator-playground](https://github.com/NARUBROWN/spine-validator-playground)는 이 계약의 실제 go-playground 구현입니다. `playground.New()`로 생성한 검증기를 `spine.WithValidator`에 전달합니다. 현재 구현의 go-playground v10.30.6은 Go 1.26 이상이 필요하며 Spine 코어의 최소 버전과 별도로 관리됩니다.

새 구현은 이 패키지를 import하고 라이브러리 오류를 공통 `FieldError`로 변환하면 됩니다. 공통 계약을 위한 별도 Go 모듈은 필요하지 않습니다. [플랫폼 계약 E2E](../../test/platform-e2e/README.md)에서 실제 요청과 브로커 메시지의 처리 결과를 확인할 수 있습니다.

## 요청별 외부 필드명

`WithFieldTags(ctx, tags...)`와 `FieldTags(ctx)`는 한 번의 검증에서 외부 필드명에 사용할 태그 우선순위를 전달합니다. 규칙 태그와 공유 검증기 설정은 바꾸지 않습니다. 구현체는 요청에 지정된 태그를 우선하고, 지정되지 않았으면 자신의 기본 필드명 설정을 사용합니다.

HTTP는 실제 바인딩에 따라 json, form, xml, path, query 또는 header를 선택합니다. 이 계약에는 HTTP 출처나 상태 코드를 넣지 않습니다. 코어가 HTTP 응답을 만들 때만 오류에 body, path, query, header 출처를 덧붙이며 WebSocket과 소비자의 공통 검증 오류 형식은 유지합니다.
