# Spine 테스트 실행

Go 테스트는 저장소 루트의 `test/` 아래에 모읍니다. 원본 `.go` 파일 옆에는 `_test.go` 파일을 두지 않습니다.

## 디렉터리 구조

| 경로 | 대상 |
| --- | --- |
| `test/unit/*_test.go` | 루트 App API |
| `test/unit/internal/` | 같은 경로의 커널 패키지 |
| `test/unit/contract/`, `test/unit/adapter/`, `test/unit/pkg/` | 공개 계약과 유틸리티 |
| `test/unit/cmd/demo/`, `test/unit/cmd/reference/` | 예제 모듈의 단위 테스트와 실제 네트워크 E2E |
| `test/unit/scripts/test/` | 테스트 실행기 |
| `test/*_test.go` | HTTP, WS와 이벤트 통합 테스트 |
| `test/openapi/` | OpenAPI와 실제 HTTP 동작 비교 |
| `test/platform-e2e/` | 외부 플랫폼과 브로커 계약 조합 |

예를 들어 `internal/input/parameters.go`의 테스트는 `test/unit/internal/input/parameters_test.go`에 둡니다. 기존 패키지 선언을 유지하므로 비공개 함수와 타입을 검사할 수 있습니다.

## 실행 명령

저장소 루트에서 Go 1.25.5 이상으로 실행합니다. 외부 구현체를 사용하는 개발용 모듈은 각 `go.mod`의 Go 버전과 의존성을 따릅니다.

```sh
go run ./scripts/test test ./... -count=1
go run ./scripts/test test -race ./... -count=1
go run ./scripts/test vet ./...
go run ./scripts/test list -test ./...
```

특정 패키지나 테스트 이름도 일반 Go 인자로 선택합니다.

```sh
go run ./scripts/test test ./internal/input -run '^TestBinding' -count=1
go run ./scripts/test -module cmd/demo test ./... -count=1
go run ./scripts/test -module cmd/reference vet ./...
```

`-module`은 실행할 모듈을 저장소 기준 상대 경로로 선택합니다. 기본값은 `.`입니다. `-root`는 저장소 밖에 복사한 검증 대상의 루트를 지정할 때 사용합니다. 실행기 옵션은 `test`, `vet`, `list` 앞에 적습니다. 나머지 인자는 Go 명령에 그대로 전달합니다.

통합 테스트는 기존 모듈에서도 직접 실행할 수 있습니다.

```sh
go -C test test -race ./... -count=1
go -C test/openapi test -race ./... -count=1
go -C test/platform-e2e test -race ./... -count=1
```

OpenAPI의 최신 로컬 변경 검증에는 코어, Echo 플랫폼과 validator를 연결한 go.work가 필요합니다. 공개 의존성만으로 실행할 수 있는 범위는 [OpenAPI 검증 안내](openapi/README.md)를 따릅니다. 실제 브로커가 필요한 항목은 [플랫폼 E2E 안내](platform-e2e/README.md)를 따릅니다.

## 실행 방식과 누락 방지

실행기는 Go의 `-overlay`로 `test/unit/`의 테스트를 원본 패키지 경로에 연결합니다. 테스트를 실행하는 동안에도 원본 디렉터리에 파일을 복사하거나 공개 API를 추가하지 않습니다. Go 프로세스가 끝나면 임시 overlay 파일을 제거하며, 테스트 실패는 명령 실패로 전달합니다.

루트의 `go test ./...`만 실행하면 `test/unit/`의 테스트를 검사하지 않습니다. 프레임워크 단위 테스트와 예제 E2E는 위 실행기를 사용해야 합니다. CI의 해당 테스트, vet와 모듈 경계 검사도 실행기를 사용합니다. CLI의 기본 Go 검사와 프레임워크 전체 검증은 별도로 실행합니다.

`test/unit/go.mod`는 통합 테스트 모듈이 보관용 테스트 소스를 별도 패키지로 탐색하지 않게 하는 경계입니다. 이 모듈에서 직접 `go test`나 `go mod tidy`를 실행하지 않습니다. 테스트의 의존성은 원본 모듈이 관리합니다.

루트 `test/` 밖에 `_test.go`를 추가하거나, 원본 패키지 디렉터리가 없는 위치에 테스트를 추가하면 실행기가 실패합니다. 테스트 전용 타입을 제공하는 기존 `internal/*/testdata` 패키지와 통합 테스트의 데이터 파일은 기존 위치를 유지합니다.
