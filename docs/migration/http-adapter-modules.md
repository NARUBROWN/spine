# HTTP 플랫폼 모듈 전환 안내

이 문서는 이슈 #9의 개발 브랜치 변경에 해당하며 정식 릴리스 태그 발행 전입니다. 기존 릴리스의 설치 방법과 혼동하지 마세요.

## 앱 생성

Echo 기반 앱은 별도 `spine-echo-platform` 모듈의 `platform.New()`를 사용합니다.

```go
import "github.com/NARUBROWN/spine-echo-platform"

app := platform.New()
```

플랫폼 모듈은 Spine과 Echo를 의존성으로 가져오고, 공개 실행 계약을 구현하는 EchoAdapter와 기본 구성을 제공합니다. 계약 자체는 Spine 모듈의 공개 `adapter` 패키지에 포함됩니다.

`spine.New()`는 HTTP 서버를 선택하지 않는 커널 생성자입니다. HTTP를 활성화하면서 구현을 지정하지 않으면 Validate와 Run에서 `adapter.ErrInvalidAdapter`로 실패합니다. HTTP 없는 워커는 기존처럼 spine.New()를 사용합니다.

직접 조립하거나 현재 CLI composition 분석을 사용한다면 아래처럼 구성합니다.

```go
import (
    "github.com/NARUBROWN/spine"
    "github.com/NARUBROWN/spine-echo-platform"
)

app := spine.New(platform.Option())
// 다른 구현: spine.New(spine.WithHTTPAdapter(factory))
```

현재 CLI는 spine.New(...)를 시작점으로 분석합니다. 플랫폼 생성자를 직접 쓰는 구성의 CLI 분석 지원은 별도 작업입니다. CLI 생성기의 기존 릴리스용 템플릿도 이 개발 버전으로 전환할 때 플랫폼 선택을 추가해야 합니다.

## 소스와 모듈 의존성

| 위치 | 역할 | 의존 관계 |
|---|---|---|
| spine/adapter | 공개 실행 계약 패키지 | 표준 라이브러리만 사용 |
| spine | 계약과 커널, 실행 조정 | 구체 HTTP 플랫폼을 참조하지 않음 |
| spine-echo-platform | Echo 구현과 편의 생성자 | Spine과 Echo |

소스는 [Spine](https://github.com/NARUBROWN/spine)과 [Echo 플랫폼](https://github.com/NARUBROWN/spine-echo-platform)의 두 저장소에서 관리합니다. 플랫폼 소스는 Spine 내부에 포함하지 않습니다. 로컬에서는 두 저장소를 형제 폴더에 체크아웃하며, 각 예제는 자신의 go.mod에서 커널과 플랫폼을 replace합니다.

플랫폼 go.mod는 Spine `v0.5.3-0.20261101161224-490135eda06f`을 참조하며 로컬 replace를 포함하지 않습니다. 예제는 플랫폼 `v0.0.0-20261101161437-2bbd8e7d0263`을 참조하고, 개발 중인 두 checkout을 검사하기 위한 replace를 유지합니다. CI의 플랫폼 checkout도 같은 커밋으로 고정합니다.

의존 모듈의 replace는 소비자에게 전파되지 않습니다. 공개 소스의 Go 의사버전과 정식 릴리스 태그는 구분해야 합니다. 현재 변경은 커널 기능 브랜치와 플랫폼 main의 소스 스냅샷이며, 정식 버전 태그는 아직 발행하지 않았습니다.

## 유지되는 동작과 전환점

기존 컨트롤러와 라우트 등록, 바인딩, 응답 staging, 인터셉터 순서, 제한값과 정상 종료 동작을 유지합니다. app.Transport는 계속 *echo.Echo를 전달하며 WS는 기존 엔진을 사용합니다.

Spine의 요청 store와 이벤트 버스는 커널이 소유합니다. native Echo store와 Spine context 사이에 값을 공유했다면 표준 요청 context로 전달하도록 변경해야 합니다. DTO의 param 태그는 native 파라미터를 사용하고 Spine path 인자는 커널 라우팅 결과를 사용하는 기존 차이를 유지합니다.

루트의 go test ./...는 중첩 모듈을 포함하지 않습니다. 전체 검증은 루트, ../spine-echo-platform, test, cmd/demo, cmd/reference와 examples/security-config의 6개 모듈에서 실행합니다. CI도 두 저장소를 체크아웃하고 같은 모듈 목록을 검사합니다. cmd/demo의 캐시 없는 HTTP/WS E2E는 계속 필수입니다.

## 리스너 없이 앱 준비하기

이 개발 브랜치는 `spine.Prepare(ctx, app, options)`를 제공합니다. 기존 App 인터페이스는 유지하며, 선택적 ApplicationPreparer를 구현한 앱만 준비할 수 있습니다. Spine과 플랫폼의 기본 앱은 이 기능을 지원합니다.

```go
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
```

Prepare는 DI와 경로, HTTP/WS 및 필요한 consumer readiness를 준비합니다. listener를 열거나 OS 신호를 구독하지 않으며 consumer와 custom transport의 Start도 호출하지 않습니다. 설정한 publisher와 broker readiness, custom Init은 실행될 수 있으므로 네트워크를 사용하지 않는 API로 간주해서는 안 됩니다. 준비 실패 시 성공적으로 획득한 자원만 정리합니다. 정리 대상은 프레임워크가 소유하는 HTTP/WS, 이벤트 인프라와 성공적으로 Init한 custom transport입니다. 임의 DI 객체의 Close를 자동 호출하는 기능은 추가하지 않으며 해당 자원은 애플리케이션이 관리합니다.

반환 결과의 수명과 소유권은 다음과 같습니다.

- Handler는 같은 실행 진입점을 반환합니다. HTTP가 비활성화돼 있으면 nil입니다. WS 업그레이드에 필요한 raw writer 기능은 유지합니다.
- 등록 목록과 boot 옵션은 준비 시 복사합니다. 이후 원본 앱의 등록 변경은 다음 Prepare에 반영됩니다. 생성자, 인터셉터 인스턴스와 사용자 제공 transport/dialer 자체는 복제하지 않습니다. 등록 변경과 Prepare를 동시에 실행하지 않습니다.
- 준비 context는 초기화 중 취소에 사용합니다. 성공한 결과는 명시적으로 Close해야 합니다. HTTP 요청은 원래 request context를 유지합니다.
- Close가 시작되면 Handler의 새 요청은 503을 받습니다. WS를 종료하고 HTTP 요청이 끝난 뒤 consumer, custom transport와 publisher를 정리합니다.
- Close의 context는 완료 대기만 제한합니다. 시간이 초과돼도 자원 정리는 계속됩니다. 진행 중인 요청이 사용하는 자원은 유지하며 이후 Close 호출로 완료를 다시 기다릴 수 있습니다. 완료한 정리는 반복하지 않고 정리 오류를 보존합니다.
- 외부 HTTP 서버의 listener, TLS와 timeout은 호출자가 관리합니다. Close는 외부 서버의 listener를 닫지 않습니다. 반드시 반환된 Handler로 요청을 연결해야 앱이 요청 수명을 추적할 수 있습니다.

HTTP가 없는 앱도 Prepare와 Close를 사용할 수 있습니다. 이 경우 Handler는 nil이고 준비한 custom transport에는 Init과 Stop만 실행합니다. Run과 RunContext는 같은 준비 경로를 사용하며 준비 성공 뒤 consumer, custom transport와 마지막 HTTP listener를 시작합니다. 관리 실행의 OS 신호 구독은 준비 전에 수행합니다.

nil context는 ErrInvalidContext, nil 앱과 nil 준비 결과는 ErrInvalidApplication, 기존 App 구현의 준비 미지원은 ErrPreparationUnsupported로 검사할 수 있습니다. 기존 실행은 RunContext(nil, options)의 background 동작을 유지합니다.

위 API는 현재 공개 개발 스냅샷에 포함됩니다. 설치할 때는 위에 고정한 버전이나 이를 포함하는 후속 버전을 사용하세요. CLI runtime probe의 HTTP 분석 범위가 확장되는 것은 아니며 기존 부분 probe 한계는 유지됩니다.

## 공개 스냅샷 설치와 검증

플랫폼을 설치하면 호환되는 Spine과 Echo도 의존성으로 가져옵니다. 개발 스냅샷을 재현할 때는 다음 고정 버전을 사용하세요.

```sh
go get github.com/NARUBROWN/spine-echo-platform@v0.0.0-20261101161437-2bbd8e7d0263
```

정식 릴리스가 나오기 전에는 기존 태그의 설치 방법과 구분합니다. 공개 모듈 소비자 검증은 저장소 루트에서 실행합니다.

```sh
bash scripts/check-published-demo.sh
```

이 검사는 실제 cmd/demo를 저장소 밖의 임시 모듈로 복사하고 replace와 직접 선언한 Spine 의존성을 제거합니다. 플랫폼의 전이 의존성으로 필요한 Spine 버전을 선택하는지 확인합니다. go.work와 기존 모듈 캐시 없이 고정한 공개 버전을 내려받아 모듈 정합성, vet와 demo 전체 race 테스트를 검사합니다. 기존 HTTP/WS, SIGINT와 Prepare E2E를 그대로 사용하며 임시 소스와 캐시는 검사 후 제거합니다. 전체 SIGINT 검증은 Unix에서 실행합니다. 로컬 소스 테스트와 공개 모듈 소비자 검증 결과는 별도로 기록합니다.
