# CLI 계약과 E2E를 공유하는 최소 예제

`cmd/demo`는 브로커 없이 실행하는 HTTP/WS 예제입니다. CLI와 서버, E2E는 `application.BuildApplication()`을 함께 사용합니다.
HTTP는 Echo 플랫폼, WS는 Gorilla 플랫폼을 `spine.New(platform.Option(), gorillaplatform.Option())`로 각각 선택합니다. 코어는 Gorilla에 의존하지 않습니다.
파일 업로드와 브로커 설정을 포함한 기존 예제는 [cmd/reference](../reference/README.md)에 보관합니다.

| 경로 | 확인할 동작 |
|---|---|
| `GET /health` | `{"status":"ok"}`와 준비 상태 |
| `GET /users/:id` | 숫자 경로 인자 바인딩 |
| `POST /echo` | JSON DTO 바인딩과 응답, 400/413/415 입력 오류 |
| `GET /ws/echo` | 실제 WS 업그레이드와 메시지 echo |

HTTP 본문 제한은 1024바이트입니다. WS는 기존 기본 제한과 동일 출처 정책을 사용합니다.
E2E는 WS 연결을 열어 둔 상태에서 SIGINT를 보내 정상 종료와 close frame까지 확인합니다.
시작 중 native 훅 및 HTTP 없는 custom transport Init에 신호가 도착하는 경우도 검사합니다.
Prepare E2E는 같은 앱을 외부 HTTP 서버에 연결해 실제 HTTP/WS 처리와 Close, WS close frame 및 종료 후 새 요청의 503을 검사합니다. 외부 서버 listener는 호출자가 계속 소유합니다.

## 실행

저장소 루트에서 실행합니다.

```sh
spine run --root cmd/demo
```

Go로 직접 실행할 수도 있습니다.

```sh
go -C cmd/demo run .
```

기본 주소는 `127.0.0.1:8080`입니다. CLI의 `SPINE_PORT`를 지원하며 `SPINE_DEMO_ADDRESS`가 있으면 그 주소를 우선합니다.

```sh
SPINE_DEMO_ADDRESS=127.0.0.1:9000 spine run --root cmd/demo
```

## 변경 후 필수 검증

프레임워크 구현 단위가 끝날 때마다 아래 검사를 수행합니다. E2E에는 `-count=1`을 유지하여 이전 성공 결과를 재사용하지 않습니다.

```sh
spine context --root cmd/demo --json
spine contract check --root cmd/demo --warnings-as-errors --json
spine verify --root cmd/demo --warnings-as-errors --json
spine contract probe --root cmd/demo --json
go -C cmd/demo test . -run '^TestDemoE2E_' -count=1 -timeout=90s
```

CLI 정적 계약 검사, 부분 runtime probe와 실제 네트워크 E2E 결과를 구분해서 기록합니다.
SIGINT 검사는 Unix에서 실행하며 Windows에서는 해당 E2E를 생략합니다.

## 통합 모듈과 CLI

HTTP 구성은 `spine.New(platform.Option())`으로 Echo 통합 모듈을 명시적으로 선택합니다.
이는 `platform.New()`와 같은 설정이며, 현재 CLI가 `spine.New(...)`를 시작점으로 찾는 규칙을 지킵니다.
커널의 `spine.New()`만 호출하면 HTTP 구현이 선택되지 않습니다.

예제는 독립 Go 모듈입니다. `go.mod`의 replace로 공개 계약을 포함한 현재 커널과 별도 Echo 플랫폼 모듈을 함께 사용합니다. 의존 버전은 공개 커밋의 Go 의사버전으로 고정하며 replace로 현재 소스를 검사합니다. 정식 버전 태그는 아직 발행하지 않았습니다.
저장소 루트의 `go test ./...`는 이 중첩 모듈을 검사하지 않으므로 위 E2E 명령을 별도로 실행해야 합니다.
프레임워크 자체의 전체 검사는 루트에서 `spine check --root .`로 수행합니다.

전체 검증에서는 `../spine-echo-platform`, `test`, `cmd/reference`, `examples/security-config`의 테스트도 각각 실행하고 `bash scripts/check-module-boundaries.sh`로 의존 경계를 검사합니다.

## 공개 모듈 소비자 검증

공개 개발 스냅샷을 배포할 때는 저장소 루트에서 다음 명령도 실행합니다.

```sh
bash scripts/check-published-demo.sh
```

스크립트는 실제 demo를 임시 폴더에 복사하고 두 replace와 직접 선언한 Spine 의존성을 제거합니다. 플랫폼의 전이 의존성으로 필요한 Spine 버전을 선택하는지 확인합니다. go.work와 기존 모듈 캐시를 사용하지 않고 원래 go.mod에 고정한 공개 버전을 내려받아 모듈 정합성, vet와 전체 demo race 테스트를 검사합니다. 기존 HTTP/WS, SIGINT와 Prepare E2E를 그대로 사용합니다. 검사가 끝나면 임시 복사본과 모듈 캐시를 제거합니다.

실행에는 Bash, Go와 공개 Go 모듈 다운로드를 위한 네트워크가 필요합니다. SIGINT E2E를 포함한 전체 검증은 Unix에서 실행합니다. 저장소의 go.mod와 replace는 변경하지 않습니다.
