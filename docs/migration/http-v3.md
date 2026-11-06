# HTTP v3 입력 처리와 준비 API 전환

HTTP v3에서는 Spine이 본문, 경로와 쿼리를 해석합니다. 플랫폼은 원시 요청, 응답 writer와 서버 수명을 제공합니다. 일반 DTO, 컨트롤러와 인터셉터 순서는 유지합니다.

HTTP v3는 코어 `v0.7.0-preview.3`, Echo `v0.2.0-preview.2`, net/http `v0.1.0-preview.1`부터 공개됐습니다. 이 소스의 전체 Preview 검증 조합은 코어 `v0.7.0-preview.4`, Echo `v0.2.0-preview.3`, net/http `v0.1.0-preview.2`와 [릴리스 운영](../../RELEASING.md#정책과-소스-ci)에 나열한 나머지 네 플랫폼입니다. 각 플랫폼은 같은 코어 태그로 검증합니다. HTTP v2 구현은 v3 코어에 그대로 연결할 수 없습니다.

## CLI로 생성하고 업데이트하기

Go와 네트워크 연결이 필요합니다. 기본 Go 프록시 설정에서는 Preview 브랜치 메타데이터를 원본 저장소에서 조회하므로 Git도 설치해야 합니다. 사용자 지정 프록시는 해당 설정을 따릅니다.

```sh
go install github.com/NARUBROWN/spine-cli/cmd/spine@v0.4.3-preview.4
spine new http-app --module example.com/http-app --platform nethttp
cd http-app
spine contract check
spine update --dry-run --json
```

Echo는 `--platform echo`로 선택합니다. CLI는 선택한 플랫폼과 코어의 `@preview`를 조회하고 해석한 태그 또는 pseudo-version을 go.mod에 기록합니다. 채널 내 최신 태그만 검색하는 방식은 아닙니다. 업데이트 계획의 diff와 `base_hash`를 확인한 뒤 `spine update --if-match <base_hash>`로 적용하세요. update는 앱 소스를 바꾸지 않습니다.

기존 v2 앱은 대응하는 플랫폼도 함께 업데이트하고 아래 Prepared API와 사용자 바인더 변경을 적용해야 합니다. 단일 패키지 설치 성공은 전체 릴리스 조합 완료를 뜻하지 않습니다. [전체 조합 완료 workflow](https://github.com/NARUBROWN/spine/actions/workflows/complete.yml)의 `release-manifest-<코어 태그>`에서 공개 모듈 조합과 성공 기록을 확인하세요.

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

## 관리 서버의 TLS

인증서를 읽은 `*tls.Config`를 `boot.HTTPOptions.TLS`에 지정하면 기존 `RunContext`와 `Serve` 경로에서 HTTPS를 실행합니다.

```go
certificate, err := tls.LoadX509KeyPair("server.crt", "server.key")
if err != nil {
    return err
}
return app.RunContext(ctx, boot.Options{
    Address: ":8443",
    HTTP: &boot.HTTPOptions{
        TLS: &tls.Config{Certificates: []tls.Certificate{certificate}},
    },
})
```

TLS가 nil이면 기존 평문 HTTP를 사용합니다. 주소 생략 시 TLS는 `:https`, 평문은 `:http`이며 OpenAPI 로그도 해당 scheme을 사용합니다. Prepare는 TLS 지원 여부와 정적 인증서 구성을 native 초기화 전에 검사하고 설정을 복사합니다. `GetCertificate`와 `GetConfigForClient`는 준비 단계에서 호출하지 않습니다. 개인 키 객체와 콜백의 안전한 동시 사용은 호출자 책임입니다.

준비한 어댑터의 `Serve(listener)`는 같은 기본값을 사용합니다. 실행 시 교체하려면 `Serve(listener, httpcontract.WithTLS(config))`로 전달합니다. 마지막 TLS 옵션이 우선하며 `WithTLS(nil)`과 nil 옵션은 오류입니다. `ServeTLS`는 추가하지 않습니다. 외부 `http.Server`에 handler만 연결하는 경우 TLS 설정과 실행은 외부 서버가 담당합니다.

이 변경은 HTTP 버전 선택과 ALPN 정책을 추가하지 않습니다. HTTP 스펙 개선은 별도 이슈에서 다룹니다.

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

`Serve`의 구현 시그니처는 `Serve(net.Listener, ...httpcontract.ServeOption) error`로 바뀝니다. 기존 `Serve(listener)` 호출은 유지되지만 이전 시그니처의 구현체는 수정해야 합니다. TLS를 지원하면 `Capabilities.TLS`를 선언하고 `Config.Serve`를 복사해 보관합니다. `ServeOptions.Resolve`로 준비한 기본값과 실행 옵션을 검증한 뒤 native 서버에 적용합니다. TLS 옵션 오류에서도 전달받은 리스너를 닫아야 합니다.

정식 import 경로는 `contract/http`입니다. 이전 `adapter`, `adapter/http`는 같은 v3 타입의 별칭입니다. 별칭은 v2 구현의 동작을 자동 변환하지 않습니다. fasthttp는 현재 계약 검증용이며 제품 배포는 이 변경의 범위에 포함하지 않습니다.

독립 `github.com/NARUBROWN/spine-nethttp-platform` 모듈은 독립 v3 구현입니다. `Option()` 또는 `NewFactory()`로 연결하며 `WithMiddleware`로 표준 `http.Handler` 미들웨어를 추가합니다. 먼저 전달한 미들웨어가 바깥에 배치됩니다. 기본 요청 분기는 `ServeMux`를 거치지 않습니다. `net/http.Server` 자체의 패닉 복구는 끌 수 없어 명시적인 `DisableRecover: true`를 준비 단계에서 거부합니다. CLI `v0.4.3-preview.4`는 `--platform nethttp` 생성과 v3 계약 분석 및 업데이트를 지원합니다.

## 로컬 소스 검증

코어와 v3 Echo 소스 경로를 지정해 검사합니다. 검증기는 임시 main module 설정을 사용하고 원래 go.mod/go.sum을 변경하지 않습니다. 공개 버전 조합의 검증은 아닙니다.

```sh
python3 scripts/verify-http-v3.py \
  --echo-root ../spine-echo-platform \
  --output-dir /tmp/spine-http-v3-proof
```

코어 단위 검사, Echo, HTTP/WS 통합과 runtime OpenAPI에 race와 vet를 실행합니다. 결과와 로그는 지정한 디렉터리에 저장합니다. 실브로커가 필요한 테스트는 환경이 없으면 생략합니다.

net/http 플랫폼 자체와 두 플랫폼의 실제 TCP 비교는 별도로 실행합니다.

```sh
python3 scripts/verify-http-platforms.py \
  --nethttp-root ../spine-nethttp-platform \
  --echo-root ../spine-echo-platform \
  --output-dir /tmp/spine-http-platform-proof
```

이 검증은 Go 1.25.5에서 동일한 DTO 결과, 오류와 검증 호출, multipart 파일 수명, native 후처리의 종료 대기, WebSocket과 runtime OpenAPI를 검사합니다. TLS는 실제 HTTPS 요청, 인증서 선택, 신뢰 검증 실패와 연결 해제 취소를 확인합니다. JSON 공백과 줄바꿈, HTTP/2와 성능은 비교하지 않습니다.

## 경로 값의 인코딩된 슬래시

HTTP 라우터는 원시 경로의 `/`로 세그먼트를 나눈 뒤 각 값을 한 번 디코딩합니다. `/files/:name`에서 `/files/a%2Fb`는 `name="a/b"`, `/files/a%252Fb`는 `name="a%2Fb"`로 전달됩니다. 실제 `/files/a/b`는 여전히 세그먼트가 세 개이므로 이 경로와 매칭되지 않습니다. Path DTO와 기존 path 인자는 같은 값을 사용하며 잘못된 percent 인코딩은 400으로 거부합니다.
