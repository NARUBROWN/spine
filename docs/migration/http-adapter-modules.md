# HTTP 플랫폼 모듈 전환 안내

이 문서는 이슈 #9의 미발행 개발 변경에 해당합니다. 기존 릴리스의 설치 방법과 혼동하지 마세요.

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

의존 모듈의 replace는 소비자에게 전파되지 않습니다. 현재 v0.0.0은 로컬 개발용 placeholder입니다. 실제 배포는 커널의 공개 버전을 고정한 뒤 플랫폼을 배포하고 replace 없는 소비자에서 검증합니다. 아직 모듈 릴리스 전이며 GitHub 저장소 생성과 공개 설치 가능 상태는 구분합니다.

## 유지되는 동작과 전환점

기존 컨트롤러와 라우트 등록, 바인딩, 응답 staging, 인터셉터 순서, 제한값과 정상 종료 동작을 유지합니다. app.Transport는 계속 *echo.Echo를 전달하며 WS는 기존 엔진을 사용합니다.

Spine의 요청 store와 이벤트 버스는 커널이 소유합니다. native Echo store와 Spine context 사이에 값을 공유했다면 표준 요청 context로 전달하도록 변경해야 합니다. DTO의 param 태그는 native 파라미터를 사용하고 Spine path 인자는 커널 라우팅 결과를 사용하는 기존 차이를 유지합니다.

루트의 go test ./...는 중첩 모듈을 포함하지 않습니다. 전체 검증은 루트, ../spine-echo-platform, test, cmd/demo, cmd/reference와 examples/security-config의 6개 모듈에서 실행합니다. CI도 두 저장소를 체크아웃하고 같은 모듈 목록을 검사합니다. cmd/demo의 캐시 없는 HTTP/WS E2E는 계속 필수입니다.

## Prepare API의 의미

Prepare는 리스너 없이 DI, 라우트와 파이프라인을 준비한 뒤 Handler와 자원 정리 수단을 제공하려는 후속 API입니다. 아직 구현돼 있지 않으며 플랫폼 모듈을 별도 저장소로 관리하기 위한 선행 조건은 아닙니다. 기존 이슈의 리스너 없는 앱 구성 요구사항에 해당합니다.
