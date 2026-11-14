# 임베드 OpenAPI 문서 UI

선택한 Swagger UI와 ReDoc을 Go 바이너리에 포함합니다. 앱 실행과 브라우저 문서 조회에는 npm, CLI, 소스 파일 또는 CDN 연결이 필요하지 않습니다.

## 연결

기존 HTTP 앱의 부팅 옵션에 사용할 UI를 추가하세요.

```go
import (
    "github.com/NARUBROWN/spine/pkg/boot"
    "github.com/NARUBROWN/spine/pkg/openapi/ui/redoc"
    "github.com/NARUBROWN/spine/pkg/openapi/ui/swaggerui"
)

options := boot.Options{
    Address: ":8080",
    HTTP: &boot.HTTPOptions{
        OpenAPI: boot.OpenAPIOptions{
            Enabled: true,
            Title:   "Example API",
            Version: "1.0.0",
            UI: []boot.OpenAPIUI{
                swaggerui.New(swaggerui.Options{}),
                redoc.New(redoc.Options{}),
            },
        },
    },
}
```

이 옵션을 `app.Run(options)` 또는 `app.Prepare(ctx, options)`에 전달하면 `/openapi.json`, `/docs`, `/redoc`을 제공합니다. Swagger UI만 사용하려면 ReDoc의 import와 등록을 생략하세요. 코어는 UI 패키지를 import하지 않으므로 선택하지 않은 UI의 자산은 앱의 의존성에 추가되지 않습니다.

`OpenAPI.Enabled`의 기본값은 false입니다. UI를 등록해도 이 값을 true로 바꾸지 않으면 문서와 자산을 제공하지 않습니다. `Enabled: true`만 지정하면 기존처럼 JSON 명세만 제공합니다.

코어는 `boot.OpenAPIUI` 계약으로 등록한 제공자를 호출합니다. `Prepare`와 `Run`은 사전 검사에서 제공자를 한 번 호출하고, 경로와 Content-Type을 검증한 리소스를 HTTP 어댑터에 연결합니다. 검증과 응답에는 같은 리소스를 사용합니다. 별도로 `Validate`를 호출하면 그 호출에서도 제공자를 실행하며, 이후 `Prepare`나 `Run`은 다시 검사합니다. 직접 제공자를 작성할 때는 외부 연결과 사용자 핸들러 실행 없이 정적 리소스를 반환하세요.

상세 입력 폼과 응답 모델은 앱에 연결한 validator의 설명 계약에 따라 달라집니다. validator가 없는 앱의 명세에는 경로와 메서드만 표시합니다. UI가 이 제한을 보완하거나 임의로 스키마를 추정하지 않습니다.

## 옵션과 요청 처리

각 UI의 `Options`에는 다음 필드가 있습니다.

| 필드 | Swagger UI 기본값 | ReDoc 기본값 | 의미 |
| --- | --- | --- | --- |
| `Path` | `/docs` | `/redoc` | 정적 절대 경로인 문서 주소 |
| `Title` | `API documentation` | `API reference` | 브라우저 탭 제목 |

명세 경로는 기존 `boot.OpenAPIOptions.Path`로 변경합니다. 문서와 자산 경로는 `HTTP.GlobalPrefix`와 독립적입니다. UI는 상대 URL로 명세와 자산을 읽으므로 프록시가 외부 경로에 접두사를 붙이고 서버에 전달할 때 제거하는 배치도 지원합니다. 명세에 서버 주소가 없으면 브라우저에서 읽은 배포 접두사를 기본 서버 주소로 사용합니다. 명세에 서버가 명시돼 있으면 그 선언을 유지합니다.

정적 자산과 라이선스 고지는 각 UI 경로 아래의 `/_assets/`에서 제공합니다. 문서와 자산은 GET과 HEAD만 처리하며, 업무용 Spine 인터셉터를 실행하지 않는 공개 리소스입니다. 문서 접근을 제한하는 배포에서는 JSON 명세, 두 UI 경로와 그 아래 자산을 같은 프록시 접근 정책으로 보호하세요. Swagger에서 실행한 API 요청에는 원래 API의 인터셉터가 적용됩니다.

앱의 GET, HEAD 또는 WebSocket 경로가 문서나 자산 경로와 겹치면 `OPENAPI_PATH_CONFLICT`로 준비를 거부합니다. 경로 인자와의 겹침도 검사합니다. 중복 UI 경로, 잘못된 경로, nil 제공자와 잘못된 Content-Type도 준비 전에 거부합니다.

Swagger UI의 외부 명세 검증과 인증 정보 영구 저장은 비활성화합니다. ReDoc은 Google Fonts를 요청하지 않으며 시스템 글꼴을 사용합니다. ReDoc 번들의 CDN 로고 URL은 제공 시 `data:,`로 대체해 원래 UI의 텍스트 대체 표시를 사용합니다. Redocly 출처 링크와 저작권 고지는 유지합니다. 앱의 OpenAPI 문서 자체에 외부 이미지나 외부 `$ref`를 추가하면 해당 URL에는 별도 접근이 필요합니다.

## 라이선스와 재배포

| 배포본 | 포함 버전 | 라이선스 |
| --- | --- | --- |
| `swagger-ui-dist` | `5.34.1` | Apache-2.0 |
| `redoc` | `2.5.4` | MIT |

두 라이선스는 고지 조건을 지키는 임베드와 재배포를 허용합니다. 원본 UI 자산은 상위 배포본의 바이트를 그대로 보관합니다. 각 패키지의 `assets/`에 상위 LICENSE, 존재하는 NOTICE와 번들 라이선스 고지를 보관하고 바이너리에도 포함합니다. 화면 아래의 링크에서 원문을 읽을 수 있습니다. 바이너리를 배포할 때는 사용하는 UI의 라이선스와 고지를 제품의 재배포 자료에도 포함하세요.

`THIRD_PARTY_LICENSES.txt`에는 원래 번들 고지와 고정한 소스 패키지의 런타임 의존성 라이선스 원문을 함께 보관합니다. 후자는 배포 번들보다 넓은 보완 목록이며 정확한 번들 SBOM으로 해석하면 안 됩니다. 별도 LICENSE가 없는 패키지는 원래 package.json, README의 저작권 고지와 선언된 표준 라이선스 원문을 보관합니다. 출처에서 보완한 원문과 메타데이터가 불명확했던 기존 라이선스는 버전과 해시를 고정했습니다.

Spine 코드의 MIT 라이선스와 각 외부 자산의 라이선스는 각각 유지합니다.

- [Swagger UI LICENSE](https://github.com/swagger-api/swagger-ui/blob/main/LICENSE)
- [ReDoc LICENSE](https://github.com/Redocly/redoc/blob/main/LICENSE)
- [배포본과 파일 해시](../../../scripts/openapi-ui/manifest.json)

## 버전 업데이트

업데이트 작업에만 Node.js, npm과 Python 3이 필요합니다. 저장소 루트에서 실행하세요.

```sh
go run ./scripts/openapi-ui --check-upstream
go run ./scripts/openapi-ui --update
go run ./scripts/openapi-ui --check
python3 -m unittest discover -s scripts/openapi-ui -p 'test_*.py' -v
node --test scripts/openapi-ui/test_bundle.cjs
go run ./scripts/test test ./... -count=1
go -C test test . -run '^TestOpenAPI' -count=1
```

Go 통합 테스트의 플랫폼 의존성 구성은 [테스트 안내](../../../test/README.md)를 따릅니다. 업데이트 후에는 실제 브라우저에서도 외부 요청을 차단하고 두 화면의 렌더링과 Swagger API 호출을 확인하세요.

업데이트 스크립트는 npm의 설치 스크립트를 실행하지 않습니다. 공식 npm registry의 SHA-512 무결성을 확인한 뒤, 고정 버전과 파일 SHA-256을 manifest에 기록합니다. 고정 배포본을 다시 가져오려면 `--refresh`를 사용하세요. CI의 `--check`는 네트워크 없이 자산, 고지와 입력 파일의 변경을 검사합니다.

Swagger UI 5.34.1에는 base64url 예제, 지수 표기의 multipleOf 표시와 contains 예제 생성을 고치는 로컬 패치가 있습니다. `scripts/openapi-ui/swagger-ui-fixes.json`의 패치를 `--refresh`에서 다시 적용하며 원본 배포본의 무결성과 라이선스 고지는 보존합니다. 다른 버전으로 업데이트할 때는 패치의 필요성과 적용 위치를 재검토해야 합니다. 버전이나 패치 대상이 달라지면 업데이트를 중단합니다.

자동 업데이트 범위는 Swagger UI 5.x와 ReDoc 2.x의 안정 버전입니다. 라이선스 변경이나 미검토 라이선스는 업데이트를 중단합니다. 새 메이저는 `--check-upstream` 결과에 별도로 표시합니다. 현재 ReDoc 래퍼는 2.x의 `Redoc.init` API를 사용하므로 3.x 전환에는 별도 통합과 브라우저 검증이 필요합니다.
