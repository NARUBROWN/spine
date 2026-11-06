# HTTP v3 입력 처리와 준비 API 전환

HTTP v3에서는 Spine이 본문, 경로와 쿼리를 해석합니다. 플랫폼은 원시 요청, 응답 writer와 서버 수명을 제공합니다. 일반 DTO, 컨트롤러와 인터셉터 순서는 유지합니다.

이 문서는 이슈 #28 작업 브랜치의 소스 변경을 설명합니다. 로컬 후보는 코어 `v0.7.0-preview.3`, Echo `v0.2.0-preview.2`, net/http `v0.1.0-preview.1`입니다. v3에 맞춘 코어와 플랫폼을 함께 사용해야 하며 공개된 v2 플랫폼은 그대로 연결할 수 없습니다. 새 조합의 태그, CLI 생성 지원과 공개 설치 검증은 아직 없습니다.

## 외부 HTTP 서버 연결

`PreparedApplication.Handler()`를 `spine.NetHTTPHandler(prepared)`로 바꿉니다. helper는 HTTP 비활성화와 net/http 기능 미지원을 오류로 알립니다.

```go
prepared, err := app.Prepare(ctx, boot.Options{HTTP: &boot.HTTPOptions{}})
if err != nil {
    return err
}
handler, err := spine.NetHTTPHandler(prepared)
if err != nil {
    _ = prepared.Close(context.Background())
    return err
}
server := &http.Server{Handler: handler}
```

외부 서버는 호출자가 종료하고 앱 자원은 `prepared.Close(ctx)`로 정리합니다. `prepared.HTTPAdapter()`는 동일한 준비 어댑터를 반환합니다. 소유권은 준비 결과에 있으므로 별도로 Serve나 Shutdown을 중복 호출하지 않습니다.

Close는 어댑터의 StopAccepting을 먼저 호출해 새 진입을 차단합니다. 이후 WS 연결을 종료하고 native middleware 후처리까지 포함한 HTTP 요청이 끝나면 나머지 앱 자원을 정리합니다. Close의 context는 대기만 제한하며 정리는 계속됩니다.

## 본문과 파일 처리

Spine은 JSON, XML, urlencoded form과 multipart를 같은 파서로 처리합니다. URL 쿼리와 경로 값을 본문에 합치지 않습니다. 전역과 라우트 인증 인터셉터는 본문 파싱 전에 실행하며, DTO 하나의 바인딩이 성공한 직후 해당 DTO를 검증합니다.

| 설정 | 0의 의미 | 그 외 값 |
| --- | --- | --- |
| `boot.HTTPOptions.MaxBodyBytes` | 기존 32 MiB 한도 | 양수는 reader의 전체 본문 한도, 음수는 제한 해제 |
| `boot.HTTPOptions.MultipartMemoryBytes` | 파일 메모리 예산 32 MiB | 양수는 파일 메모리 예산, 음수는 준비 오류 |

본문 한도는 플랫폼이 적용합니다. 코어 파서는 같은 한도를 다시 적용하지 않습니다. multipart 메모리 예산은 전체 요청 메모리 사용량의 상한이 아니며, 예산을 넘은 파일은 디스크로 넘깁니다. native timeout과 헤더 옵션의 0은 플랫폼 기본값으로 전달하며, 명시한 미지원 옵션은 초기화 전에 `ErrUnsupportedOption`으로 거부합니다.

폼 DTO와 UploadedFiles는 같은 multipart 결과를 공유하므로 인자의 순서를 바꿔도 결과가 같습니다. 파일은 모든 AfterCompletion이 끝날 때까지 사용할 수 있습니다. Execute가 반환하기 전에 임시 파일을 제거하므로 native middleware의 후처리나 요청 이후 작업에서 파일이 필요하면 완료 훅이 끝나기 전에 별도 저장소로 복사하세요.

400, 413, 415와 `in: body` 오류 출처는 유지합니다. 취소와 원래 I/O 오류는 원인을 보존하며 일괄 400으로 바꾸지 않습니다. 임의의 reader에서 이미 막힌 읽기를 강제로 중단하지 않으며 소켓 읽기 종료와 기한은 플랫폼이 관리합니다.

## Echo 확장 이행

`WithEcho`의 native 라우트와 middleware는 유지합니다. Spine DTO의 입력 해석은 Echo Binder/JSONSerializer, `UnmarshalParam`, `UnmarshalParams`와 `format` 태그를 호출하지 않습니다. 사용자 디코더는 표준 `json.Unmarshaler`, XML 디코더와 `encoding.TextUnmarshaler`로 옮기거나 native Echo 라우트에서 사용하세요. Echo의 모든 바인딩 확장과 동등한 동작을 보장하지 않습니다.

Echo의 응답 출력 경로와 사용자 JSONSerializer는 유지합니다. writer는 코어가 직렬화한 `json.RawMessage`를 받습니다. 응답 staging과 BeforeResponse 이후 flush 순서는 바뀌지 않습니다.

## 플랫폼 구현 변경

[`contract/http`](../../contract/http/README.md)의 필수 요청 입력은 Context, Method, RawPath, RawQuery, Header(s), Body와 Writer입니다. Bind, MultipartForm과 라우팅 결과는 코어의 요청 뷰가 구현합니다. net/http handler와 기존 WS v1의 업그레이드는 선택 기능으로 선언합니다.

서버는 Serve, StopAccepting과 Shutdown을 구현합니다. 가장 바깥 native handler에서 Spine 경로, native 경로와 업그레이드를 함께 추적하고, Shutdown의 대기 시간이 끝나도 정리를 계속해야 합니다. 정상 Serve 종료는 `httpcontract.ErrServerClosed`로 정규화합니다.

정식 import 경로는 `contract/http`입니다. 이전 `adapter`, `adapter/http`는 같은 v3 타입의 별칭입니다. 별칭은 v2 구현의 동작을 자동 변환하지 않습니다. fasthttp는 현재 계약 검증용이며 제품 배포는 이 변경의 범위에 포함하지 않습니다.

독립 `github.com/NARUBROWN/spine-nethttp-platform` 모듈은 로컬 v3 구현입니다. `Option()` 또는 `NewFactory()`로 연결하며 `WithMiddleware`로 표준 `http.Handler` 미들웨어를 추가합니다. 먼저 전달한 미들웨어가 바깥에 배치됩니다. 기본 요청 분기는 `ServeMux`를 거치지 않습니다. `net/http.Server` 자체의 패닉 복구는 끌 수 없어 명시적인 `DisableRecover: true`를 준비 단계에서 거부합니다. CLI 생성과 공개 설치 조합은 아직 제공하지 않습니다.

## 로컬 소스 검증

코어와 v3 Echo 소스 경로를 지정해 검사합니다. 검증기는 임시 main module 설정을 사용하고 원래 go.mod/go.sum을 변경하지 않습니다. 공개 버전 조합의 검증은 아닙니다.

```sh
python3 scripts/verify-http-v3.py \
  --echo-root ../spine-echo-issue-28 \
  --output-dir /tmp/spine-http-v3-proof
```

코어 단위 검사, Echo, HTTP/WS 통합과 runtime OpenAPI에 race와 vet를 실행합니다. 결과와 로그는 지정한 디렉터리에 저장합니다. 실브로커가 필요한 테스트는 환경이 없으면 생략합니다.

net/http 플랫폼 자체와 두 플랫폼의 실제 TCP 비교는 별도로 실행합니다.

```sh
python3 scripts/verify-http-platforms.py \
  --nethttp-root ../spine-nethttp-platform \
  --echo-root ../spine-echo-issue-28 \
  --output-dir /tmp/spine-http-platform-proof
```

이 검증은 Go 1.25.5에서 동일한 DTO 결과, 오류와 검증 호출, multipart 파일 수명, native 후처리의 종료 대기, WebSocket과 runtime OpenAPI를 검사합니다. JSON 공백과 줄바꿈, TLS, HTTP/2와 성능은 비교하지 않습니다.
