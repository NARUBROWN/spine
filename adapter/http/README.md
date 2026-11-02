# HTTP 어댑터 실행 계약

`github.com/NARUBROWN/spine/adapter/http`는 Spine 커널과 플랫폼 구현이 공유하는 공개 패키지입니다. Spine 모듈에 포함되며 버전도 Spine과 함께 관리합니다. 별도 계약 모듈이나 저장소는 사용하지 않습니다.

계약 패키지는 표준 라이브러리만 import합니다. Spine의 내부 파이프라인, 요청 store, 이벤트 버스와 구체 HTTP 프레임워크에 의존하지 않습니다. 외부 플랫폼 모듈은 이 공개 패키지를 import하여 Factory와 HTTPAdapter를 구현할 수 있습니다.

Factory는 기능을 선언하고 리스너 없이 새 어댑터를 구성합니다. HTTPContext는 요청 입력과 실제 ResponseWriter를 제공하며, ExecuteFunc는 요청마다 커널을 한 번 동기 실행합니다. Handler는 외부 HTTP 서버에 연결할 수 있고 Serve와 Shutdown은 관리 서버 실행에 사용합니다.

현재 계약 버전은 `spine.http-adapter/v2`입니다. `ResponseWriter.WriteJSON`은 core에서 직렬화한 `json.RawMessage`만 받습니다. DTO의 동적 타입이 필요한 `HTTPContext.Bind(any)`가 공개 HTTP 계약에 남는 유일한 `any`입니다.

native Echo 설정은 플랫폼의 `WithEcho(func(*echo.Echo))` 옵션이 소유합니다. 공통 Config와 Capabilities에는 native hook을 넣지 않습니다. v1 구현은 버전 검사에서 거부하므로 커널과 플랫폼을 함께 전환해야 합니다.

scope와 writer는 요청 중에만 사용하며 별도 goroutine에서 동시 접근하거나 보관하지 않습니다. Spine의 요청 store, 라우팅 결과, EventBus와 응답 staging은 커널이 소유합니다. 요청 오류는 RequestError로 상태와 원인을 전달하고 커널이 공개 HTTP 오류로 변환합니다.

ASGI의 앱과 서버 경계 및 수명 정의를 참고하지만 **Python ASGI 호환을 제공하지 않습니다.** Spine v0.6.0과 Echo 플랫폼 v0.1.0에서 이 계약을 사용합니다.

```sh
go test ./adapter/http -count=1
```
