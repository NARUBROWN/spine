# Echo 플랫폼으로 전환하기

Echo 기반 앱은 `spine-echo-platform`을 설치하고 `platform.New()`로 생성합니다. 플랫폼이 Spine과 Echo를 함께 가져오며, HTTP 실행 계약은 Spine 모듈의 공개 `adapter` 패키지에서 관리합니다.

이 문서는 [이슈 #9](https://github.com/NARUBROWN/spine/issues/9)와 [PR #10](https://github.com/NARUBROWN/spine/pull/10)의 공개 개발 스냅샷을 기준으로 합니다. 정식 릴리스 태그는 아직 발행하지 않았습니다.

## 플랫폼 설치와 앱 생성

애플리케이션의 Go 모듈에서 다음 버전을 설치합니다.

```sh
go get github.com/NARUBROWN/spine-echo-platform@v0.0.0-20261101161437-2bbd8e7d0263
```

이 버전은 Prepare API를 포함한 Spine `v0.5.3-0.20261101161224-490135eda06f`에 의존합니다. 플랫폼을 설치하면 필요한 Spine과 Echo 버전도 선택됩니다. 별도 계약 모듈을 설치하거나 로컬 `replace`를 추가할 필요는 없습니다.

앱 생성 코드를 다음과 같이 바꿉니다. 기존 생성자, 컨트롤러와 라우트 등록은 이어서 사용합니다.

```go
import platform "github.com/NARUBROWN/spine-echo-platform"

app := platform.New()
```

직접 조립할 때는 같은 구성을 명시할 수 있습니다.

```go
import (
    "github.com/NARUBROWN/spine"
    platform "github.com/NARUBROWN/spine-echo-platform"
)

app := spine.New(platform.Option())
```

`spine.New()`는 HTTP 구현을 선택하지 않는 커널 생성자입니다. HTTP를 활성화하면서 factory를 지정하지 않으면 Validate, Prepare와 Run에서 `adapter.ErrInvalidAdapter`로 실패합니다. HTTP 없는 워커는 `spine.New()`를 사용합니다. 다른 구현을 연결할 때는 `spine.WithHTTPAdapter(factory)`로 주입합니다.

현재 CLI로 분석할 앱은 `spine.New(platform.Option())` 형태를 사용합니다. CLI가 `spine.New(...)`를 composition 시작점으로 찾기 때문입니다. `platform.New()`의 직접 분석과 기존 CLI 생성 템플릿의 플랫폼 선택 반영은 후속 작업입니다.

## 계약과 구현의 책임

| Go 패키지 | 책임 | 배포 위치 |
|---|---|---|
| `adapter` | 요청 입력, 실제 응답 writer, factory, 기능 및 서버 수명 계약 | Spine 모듈 |
| `spine` | DI, 라우팅, 요청 store, EventBus, 파이프라인, 응답 staging과 실행 조정 | Spine 모듈 |
| `platform` | EchoAdapter 구현, DTO/multipart 바인딩, 실제 HTTP 출력과 편의 생성자 | Echo 플랫폼 모듈 |
| `echo` | native 훅에서 사용하는 Echo API | Labstack Echo 모듈 |

계약 소스는 [Spine 저장소](https://github.com/NARUBROWN/spine)의 공개 `adapter` 패키지에 있고 커널과 함께 버전을 관리합니다. 별도 계약 저장소나 Go 모듈은 사용하지 않습니다. 계약 패키지는 표준 라이브러리만 참조하며, 커널은 Echo와 플랫폼 모듈을 import하지 않습니다.

구현 소스는 별도 [Echo 플랫폼 저장소](https://github.com/NARUBROWN/spine-echo-platform)에서 관리합니다. Go 패키지 이름은 `platform`이고, native Labstack Echo의 패키지 이름은 `echo`입니다. 기존 서버 API를 Spine 계약에 맞게 연결하는 책임은 플랫폼에 있습니다.

ASGI의 앱과 서버 분리, 요청과 연결의 수명 및 시작과 종료 책임을 참고합니다. **이번 작업은 Python ASGI와의 호환을 의미하지 않습니다.** 현재 HTTP 구현은 Echo 플랫폼이며, 자체 `net/http` 플랫폼 구현은 후속 작업입니다.

## 리스너 없이 앱 준비하기

`spine.Prepare(ctx, app, options)`는 준비한 앱의 `Handler()`와 `Close(ctx)`를 반환합니다. 기존 App 인터페이스는 유지하며, 선택적 `ApplicationPreparer`를 구현한 앱이 이 기능을 제공합니다. `spine.New(...)`와 `platform.New()`로 만든 기본 앱은 이를 지원합니다.

아래는 `/health` 라우트가 등록된 앱을 테스트하는 예입니다. HTTP를 사용하려면 `boot.Options.HTTP`를 지정해야 합니다.

```go
options := boot.Options{HTTP: &boot.HTTPOptions{}}
prepared, err := spine.Prepare(context.Background(), app, options)
if err != nil {
    t.Fatal(err)
}
t.Cleanup(func() {
    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()
    if err := prepared.Close(ctx); err != nil {
        t.Error(err)
    }
})

recorder := httptest.NewRecorder()
prepared.Handler().ServeHTTP(recorder, httptest.NewRequest("GET", "/health", nil))
if recorder.Code != http.StatusOK {
    t.Fatalf("status = %d", recorder.Code)
}
```

Prepare와 준비 결과의 수명 규칙은 다음과 같습니다.

- **준비:** DI, 경로와 HTTP/WS 및 필요한 consumer readiness를 구성합니다. 프레임워크의 listener와 OS 신호 구독, consumer와 custom transport의 Start는 실행하지 않습니다. publisher 구성, broker readiness와 custom Init은 실행될 수 있으며 설정에 따라 네트워크 연결이 발생할 수 있습니다.
- **설정과 context:** 등록 목록과 boot 옵션을 복사합니다. 이후 원본 등록의 변경은 다음 Prepare에 반영됩니다. 생성자와 인터셉터 인스턴스, 사용자 제공 transport/dialer 자체는 복제하지 않습니다. 등록 변경과 Prepare를 동시에 실행하지 않습니다. 준비 context는 초기화 중 취소에 사용하고 HTTP 요청은 원래 request context를 유지합니다.
- **Handler:** 같은 실행 진입점을 반환합니다. HTTP가 비활성화돼 있으면 nil입니다. 외부 서버와 연결할 때는 반환된 Handler를 사용해야 앱이 요청 수명을 추적할 수 있습니다. WS 업그레이드에 필요한 raw writer 기능도 유지합니다.
- **Close:** 새 요청을 503으로 거부하고 WS를 종료합니다. 진행 중인 HTTP 요청이 끝난 뒤 consumer, custom transport와 publisher를 정리합니다. 호출 context는 완료 대기만 제한하므로 시간이 초과돼도 정리는 계속됩니다. 이후 Close로 완료를 다시 기다릴 수 있습니다. 완료한 정리를 반복하지 않고 정리 오류를 보존합니다.
- **소유권:** 준비에 실패하면 성공적으로 획득한 프레임워크 자원을 정리합니다. 준비에 성공하면 호출자가 Close해야 합니다. 외부 HTTP 서버의 listener, TLS와 timeout 및 임의 DI 객체의 자원은 호출자가 관리합니다. Close는 외부 서버의 listener를 닫지 않습니다.

HTTP 없는 앱도 Prepare와 Close를 사용할 수 있습니다. 이 경우 Handler는 nil이고 준비한 custom transport에는 Init과 Stop을 호출합니다. Run과 RunContext는 같은 준비 경로를 사용하며, 준비에 성공한 뒤 consumer와 custom transport를 시작하고 마지막으로 HTTP listener를 엽니다. 관리 실행의 OS 신호 구독은 준비 전에 수행합니다.

nil context는 `ErrInvalidContext`, nil 앱과 nil 준비 결과는 `ErrInvalidApplication`, 외부 App 구현의 준비 미지원은 `ErrPreparationUnsupported`로 검사할 수 있습니다. 기존 `RunContext(nil, options)`의 background 동작은 유지합니다.

## 유지되는 동작과 native 확장

바인딩과 응답 형식, 응답 staging, 인터셉터 순서, 제한값과 정상 종료 동작을 유지합니다. staging은 커널에 남고 실제 HTTP 출력은 플랫폼이 처리합니다. WebSocket 업그레이드는 공개 계약으로 연결하며 Gorilla 메시지 엔진과 연결 종료는 커널에 유지합니다.

`app.Transport`는 계속 `*echo.Echo`를 전달합니다. native Echo 경로는 Spine의 HTTP 인터셉터, 본문 제한과 staging을 거치지 않습니다. native Echo store와 Spine 요청 store는 분리하므로 공유 값은 표준 request context로 전달합니다. DTO의 `param` 태그는 native 파라미터를, Spine path 인자는 커널 라우팅 결과를 사용하는 기존 차이도 유지합니다.

## 프레임워크 소스 개발과 검증

두 저장소를 함께 수정할 때는 형제 폴더에 체크아웃합니다.

```text
VscodeProjects/
  spine/
  spine-echo-platform/
```

Spine 저장소의 `test`, `cmd/demo`, `cmd/reference`와 `examples/security-config`는 각각 별도 Go 모듈입니다. 이 개발용 모듈의 `replace`는 위 두 checkout을 검사하기 위한 설정입니다. 공개 플랫폼의 go.mod는 공개 Spine 버전을 사용하고 `replace`를 포함하지 않습니다. 의존 모듈의 `replace`는 소비자 앱에 전파되지 않으므로 공개 버전은 별도 소비자에서 검증합니다. 로컬 `go.work`는 커밋하지 않습니다.

루트의 `go test ./...`는 중첩 모듈을 포함하지 않습니다. 루트와 플랫폼, 위 4개 개발용 모듈을 각각 검사합니다. HTTP/WS E2E는 계속 실제 `cmd/demo`를 기준으로 실행합니다.

```sh
bash scripts/check-module-boundaries.sh
spine context --root cmd/demo --json
spine contract check --root cmd/demo --warnings-as-errors --json
spine verify --root cmd/demo --warnings-as-errors --json
spine contract probe --root cmd/demo --json
go -C cmd/demo test . -run '^TestDemoE2E_' -count=1 -timeout=90s
```

CLI 정적 분석, 부분 runtime probe와 실제 E2E는 별도 검증 결과입니다. Prepare 추가가 CLI probe의 HTTP 실행 분석 범위를 확장하는 것은 아닙니다.

공개 버전의 설치와 실행은 다음 명령으로 따로 검사합니다.

```sh
bash scripts/check-published-demo.sh
```

이 검사는 실제 demo를 저장소 밖으로 복사하고 `replace`와 직접 선언한 Spine 의존성을 제거합니다. `go.work`와 기존 모듈 캐시 없이 플랫폼을 설치해 필요한 Spine 버전이 선택되는지 확인합니다. 같은 demo 전체의 vet/race와 HTTP/WS, SIGINT 및 Prepare E2E를 실행하며, 임시 소스와 캐시는 검사 후 제거합니다. 전체 SIGINT 검증은 Unix에서 실행합니다.

## 검증된 범위와 남은 작업

[PR 커밋 93cfac1](https://github.com/NARUBROWN/spine/commit/93cfac157468c54a0f2ceccbf243fbaff7792007)의 [원격 CI 10개 검사](https://github.com/NARUBROWN/spine/actions/runs/41327017512)가 통과했습니다. 새 clone과 Go 1.25.5 / Linux arm64 컨테이너에서도 6개 모듈과 CLI, 공개 소비자 demo를 검증했습니다. 실브로커가 필요한 Kafka/RabbitMQ 테스트 2개는 환경이 없어 생략했습니다.

독립 검토에서 CI의 플랫폼 단독 vet/unit/race가 PR checkout 대신 고정된 공개 Spine 버전을 검사한다는 보완 지점을 확인했습니다. 현재 공개 pin과 위 PR 커밋의 런타임 소스는 같습니다. 이후 커널 변경의 회귀도 검출하려면 플랫폼 검사에만 임시 Go workspace로 PR 소스를 연결해야 합니다. 모듈 정합성과 공개 소비자 검사는 `GOWORK=off`를 유지합니다. 이 CI 보완은 아직 반영하지 않았습니다.

PR 리뷰 승인과 main 병합, 정식 릴리스 태그 발행도 남아 있습니다.
