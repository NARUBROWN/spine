# HTTP v3 계약

플랫폼은 원시 요청 입력과 응답 출력을 제공하고, Spine은 DTO 파싱과 실행 규칙을 소유합니다. 필수 계약은 net/http나 fasthttp의 native 요청 및 handler 타입을 요구하지 않습니다.

`github.com/NARUBROWN/spine/contract/http`는 커널과 플랫폼이 공유하는 `spine.http-adapter/v3` 계약입니다. 이 브랜치의 커널은 v3를 사용합니다. 공개된 v2 플랫폼과 함께 사용할 수 없으며, 새 조합은 아직 발행하지 않았습니다. 초기 검증 경로인 `contract/http/v3`와 이전 `adapter`, `adapter/http`는 같은 타입과 오류의 별칭입니다.

## 입력과 출력

`HTTPContext`는 `Context`, `Method`, `RawPath`, `RawQuery`, `Header`, `Headers`, `Body`, `Writer`를 제공합니다. 플랫폼의 `Bind`, `MultipartForm`, `Param/Params`, `Query/Queries`는 v3 필수 계약에 없습니다. 컨트롤러에 제공하는 해당 기능은 코어가 구현합니다.

| 입력 | 보장 |
| --- | --- |
| `RawPath` | 쿼리와 authority를 제외한 디코딩 전 경로입니다. native 엔진의 경로 정리 전 값을 제공합니다. |
| `RawQuery` | 선행 `?`가 없는 원문입니다. 반복 값, 빈 값과 `%2C`를 파싱한 값으로 대체하지 않습니다. |
| `Headers` | Host와 포트를 보존하며 맵과 슬라이스 수정이 원본 요청에 영향을 주지 않습니다. native 버퍼 재사용으로 반환 값이 바뀌지 않습니다. |
| `Body` | nil이 아닌 reader이며 같은 읽기 위치를 유지합니다. getter가 추가 읽기, native 폼 파싱과 재생을 시작하지 않습니다. 원본 스트림은 플랫폼이 정리합니다. |
| `Context` | 요청 종료 때 취소하는 안전한 표준 context입니다. 풀링된 native 객체를 그대로 노출하지 않습니다. 연결 해제 취소 지원은 별도로 선언합니다. |

코어가 인증 뒤 본문 파싱을 시작하도록 플랫폼이 선행 DTO/폼 파싱을 하지 않습니다. 서버가 이미 본문을 수신했거나 native HTTP 파서가 요청을 거부하는 상황은 Spine 파싱과 구분합니다. 요청 뷰, reader와 writer는 요청 안에서 동기 사용합니다.

`ResponseWriter`는 기존 메서드를 유지합니다. `WriteJSON`은 직렬화된 `json.RawMessage`를 받고 `WriteBytes`와 함께 반환 뒤 입력 bytes를 재사용할 수 있어야 합니다. `IsCommitted`와 성공 반환은 논리적 응답 확정이며 실제 송신 또는 클라이언트 수신 완료를 뜻하지 않습니다. 관측 가능한 쓰기 오류는 반환합니다.

## 실행과 종료

| 메서드 | 보장 |
| --- | --- |
| `Serve(net.Listener)` | 리스너를 소유하고 모든 반환 경로에서 닫습니다. 한 번만 실행하고 정상 종료를 `ErrServerClosed`로 정규화합니다. 예상하지 못한 nil 반환은 계약 위반입니다. |
| `StopAccepting()` | 새 실행 진입을 동기 차단하고 반복 호출할 수 있습니다. 기존 요청은 보존하며 차단 뒤 handler에 도착한 요청에는 503을 반환합니다. |
| `Shutdown(context.Context)` | 차단과 진행 요청의 종료를 기다립니다. 호출 context는 대기만 제한하고 정리는 계속합니다. 새 context로 다시 기다릴 수 있으며 인스턴스를 재실행하지 않습니다. |

구현체는 가장 바깥 native handler에서 Spine 경로, native middleware, native 경로와 업그레이드 진입을 함께 추적합니다. 관리 서버와 외부 서버는 같은 보호된 handler를 사용합니다. 외부 서버 자체는 호출자가, 장기 WS 연결은 코어 런타임이 종료합니다. graceful 종료 시작만으로 진행 요청을 일괄 취소하지 않습니다.

## 선택 기능과 검사 순서

`Capabilities`는 계약 버전, `NetHTTPHandler`, `NetHTTPUpgrade`, `ClientDisconnectCancellation`, `ServerOptions` 지원을 선언합니다. 기본 HTTP 처리와 본문 읽기 제한은 필수입니다. net/http native 진입점이 없는 엔진도 기본 계약을 구현할 수 있습니다.

1. `ValidateFactory`로 버전과 필요한 기능을 검사합니다. 이 함수는 `New`와 native 훅을 호출하지 않습니다.
2. `Config.Validate`로 Execute, 옵션 값과 명시적으로 요청한 옵션의 지원 여부를 검사합니다.
3. net/http 업그레이드가 있으면 `ValidateNetHTTPUpgradeRoutes`로 누락 경로와 nil handler를 검사합니다.
4. 일반 HTTP는 `Factory.New`, 기존 net/http WS는 선택 `NetHTTPUpgradeFactory.NewWithNetHTTPUpgrades`를 호출합니다. 구현체도 native 훅 실행 전에 구성을 검증해야 합니다.
5. `ValidateAdapter`로 nil 결과와 선언한 native handler를 검사합니다. 실행, 차단과 종료는 시작하지 않습니다.

`NetHTTPHandler(adapter)`는 선택 `NetHTTPHandlerProvider`를 조회합니다. 지원하지 않는 엔진은 `ErrUnsupportedCapability`, nil adapter나 nil native handler는 `ErrInvalidAdapter`를 반환합니다. `PreparedApplication.HTTPAdapter()`는 동일한 준비 어댑터를 반환합니다. 외부 net/http 서버에는 `spine.NetHTTPHandler(prepared)`로 얻은 handler를 연결합니다. HTTP 비활성화는 `ErrUnsupportedCapability`로 알립니다. 실행과 종료 소유권은 Prepared에 남습니다.

`NetHTTPUpgrade`를 선언한 factory는 `NetHTTPUpgradeFactory`를 구현해야 합니다. 선언은 있으나 인터페이스가 없으면 잘못된 구현입니다. 기능을 선언하지 않은 엔진에 기존 net/http WS를 요청하면 미지원 조합입니다. 두 경우 모두 준비 단계에서 거부하며 호환 변환기를 자동 설치하지 않습니다.

## 옵션 값과 지원 범위

`ServerOptions.Validate`는 값만 검사하고 `Config.Validate`는 지원 여부까지 검사합니다. 계약 자체는 기본값을 적용하지 않습니다. 커널은 기존 `boot.HTTPOptions.MaxBodyBytes`의 0을 32 MiB로 정규화하고, native timeout과 헤더 옵션의 0은 플랫폼 기본값으로 전달합니다. 지원 선언은 요청한 의미를 구현할 수 있을 때만 켭니다.

| 옵션 | 값과 지원 규칙 |
| --- | --- |
| `ReadHeaderTimeout` | 음수를 거부합니다. 명시적인 양수는 요청 헤더 수신 기한을 지원해야 합니다. |
| `ReadTimeout` | 음수를 거부합니다. 명시적인 양수는 헤더와 본문을 포함한 요청 읽기 기한을 지원해야 합니다. |
| `WriteTimeout` | 음수를 거부합니다. 명시적인 양수의 쓰기 기한은 요청 헤더 수신 뒤 시작하는 의미를 지원해야 합니다. handler 실행을 강제 중단하는 제한은 아닙니다. |
| `IdleTimeout` | 음수를 거부합니다. 명시적인 양수는 keepalive의 다음 요청 대기 기한을 지원해야 합니다. |
| `MaxHeaderBytes` | 음수를 거부합니다. 명시적인 양수는 요청 줄과 헤더 파싱 한도를 지원해야 합니다. |
| `DisableRecover` | true를 요청하면 native 복구 비활성화를 지원해야 합니다. |
| `MaxBodyBytes` | 본문 읽기 한도는 필수입니다. 양수는 길이를 모르는 입력에도 적용하고 음수는 해제합니다. 커널은 0을 32 MiB로 정규화합니다. |

지원하지 않는 명시적 native 옵션은 `ErrUnsupportedOption`으로 거부합니다. 필드 이름만 맞추거나 지원하지 않는 값을 무시하지 않습니다. 0인 native 옵션은 지원 요구를 만들지 않으며 플랫폼이 기본값을 적용합니다. `boot.HTTPOptions.MultipartMemoryBytes`는 코어 파서의 파일 메모리 예산이며 0은 32 MiB, 음수는 준비 오류입니다. 전체 본문 한도와 전체 요청 메모리 상한은 아닙니다.

## 오류와 검증 범위

| 분류 | 오류 |
| --- | --- |
| nil, 버전 불일치, 잘못된 지원 선언과 값 | `ErrInvalidAdapter` |
| 중복 실행, 종료 후 재실행 등 | `ErrInvalidState` |
| 선택 기능 미지원 | `ErrUnsupportedCapability` |
| 명시적인 native 옵션 미지원 | `ErrUnsupportedOption` |
| 정상 관리 서버 종료 | `ErrServerClosed` |
| 입력 오류 상태와 원래 원인 | `RequestError`의 `Status`, `Message`, `Cause` |

본문 한도 초과는 `RequestError` 상태 413으로 표현하고 `Unwrap`으로 원인을 보존합니다. 취소와 일반 I/O 오류를 일괄 400으로 바꾸지 않습니다. 코어는 본문과 파일 오류의 `in: body`, 400/413/415와 원인을 보존합니다. native 프로토콜 거부는 Execute 이전에 일어날 수 있습니다.

검증은 계약 검사, 두 엔진의 최소 구현과 실제 Spine 통합으로 나눕니다. 두 엔진 검증의 fasthttp 스트리밍은 EOF 패치를 적용한 로컬 조합입니다. 실제 Spine 통합은 이행한 Echo 소스로 HTTP/WS/OpenAPI, 인증 전 무읽기와 multipart 정리를 검사합니다. 독립 net/http 제품, CLI와 공개 모듈 조합은 후속 범위입니다.

본문의 JSON/XML/form/multipart 해석은 커널이 소유하며 Echo Binder를 호출하지 않습니다. 요청별 파서는 실제 바인딩 때 생성하고, 모든 `AfterCompletion` 뒤 Execute 바깥에서 임시 파일을 정리합니다. 자세한 이행은 [HTTP v3 전환](../../docs/migration/http-v3.md)을 참고하세요.
