# 요청 본문 파서

`Body`는 요청 하나의 JSON, XML, URL-encoded form과 multipart를 처리하는 내부 구현입니다. 본문만 읽으며 URL 쿼리와 경로 값을 폼에 합치지 않습니다. 폼 필드는 기존 `BodyFields`를 사용하고 JSON의 null 판정은 `pkg/jsonx`에 맡깁니다.

이슈 [#28](https://github.com/NARUBROWN/spine/issues/28)의 첫 구현 단계입니다. 현재 `transport.HTTPContext`는 여전히 플랫폼의 `Bind`와 `MultipartForm`에 위임합니다. 이 파서는 아직 HTTP 실행 경로에 연결하지 않았으며 공개 HTTP 계약은 `spine.http-adapter/v2`입니다. 이 변경만으로 기존 애플리케이션의 바인딩 동작이 바뀌지는 않습니다.

## 생성과 설정

```go
func NewBody(ctx context.Context, reader io.Reader, contentType string, options BodyOptions) (*Body, error)
```

생성 시에는 본문을 읽지 않습니다. `ctx`와 `reader`는 nil이 아니어야 합니다. 원래 스트림은 서버 또는 어댑터가 닫습니다.

| 설정 | 생략 시 값 | 허용 값과 의미 |
| --- | --- | --- |
| `MaxBytes` | `32 << 20` 바이트 | 양수는 전체 본문 한도, 음수는 제한 해제입니다. 읽을 때 한도를 적용하므로 Content-Length가 없는 요청도 검사합니다. |
| `MultipartMemory` | `32 << 20` 바이트 | 양수는 multipart 파일의 메모리 예산입니다. 초과 파일 데이터는 임시 파일에 저장하며 음수는 거부합니다. 전체 요청 크기 제한이나 전체 메모리 사용량의 상한은 아닙니다. |

두 값은 현재 내부 구현의 기본값입니다. 공개 `boot.HTTPOptions`의 새 설정 이름과 기본값, 어댑터와 코어 중 전체 본문 제한을 적용할 주체는 후속 통합에서 정해야 합니다.

## 바인딩과 파일 수명

- `Bind(target)`은 본문을 한 번 바인딩합니다. 성공 후 다시 호출하면 `ErrBodyConsumed`를 반환하며, 실패 후 호출하면 저장한 오류를 반환합니다. 본문을 자동 재생하지 않습니다.
- JSON은 일반 값 타입의 명시적 null을 거부하고 포인터, map, slice와 사용자 `json.Unmarshaler`의 규칙을 유지합니다. XML은 표준 `xml.Decoder.Decode`로 값을 하나 읽습니다.
- 폼은 구조체 포인터의 `form` 필드에 매핑합니다. 반복 키는 슬라이스에 보존하고 쉼표를 구분자로 나누지 않습니다. 누락된 포인터는 nil을 유지하고 명시한 빈 숫자와 bool은 0과 false로 읽습니다. `encoding.TextUnmarshaler`도 호출합니다.
- multipart의 파일 필드는 `*multipart.FileHeader`, `[]*multipart.FileHeader`, `[]multipart.FileHeader`를 지원합니다. `MultipartForm()`으로 얻는 파일 목록과 폼 DTO가 같은 파싱 결과를 공유합니다. 폼과 파일 조회의 순서를 바꿔도 스트림을 다시 파싱하지 않으며 파싱 오류도 저장합니다.
- `Cleanup()`은 자신이 파싱한 임시 파일을 한 번 정리합니다. 반복 호출은 같은 정리 결과를 반환합니다. 정리 이후 파싱과 파일 조회는 `ErrBodyClosed`로 거부하며 원래 본문 스트림은 닫지 않습니다.

실행 경로에 연결할 때는 인증 인터셉터가 요청을 허용한 뒤 바인딩하고, `AfterCompletion`까지 끝난 후 `Cleanup`을 호출해야 합니다. 요청 뒤에도 파일이 필요하면 그 전에 별도 저장소로 복사해야 합니다. 현재 파서 테스트의 정리 검증은 파이프라인의 모든 종료 경로에서 정리가 연결됐다는 증거가 아닙니다.

Echo의 custom binder, serializer, `UnmarshalParam`, `UnmarshalParams`와 `format` 태그의 이행은 아직 구현하지 않았습니다. 기존 Echo와의 동등성을 주장하기 전에 폼 키 대소문자와 사용자 디코더를 포함한 대조 검사가 필요합니다.

## 오류와 취소

| 조건 | 반환 오류 |
| --- | --- |
| 본문 구문이나 필드 변환 실패 | 상태가 400인 `httperr.HTTPError` |
| 전체 본문 한도 또는 multipart 파서의 크기 한도 초과 | 상태가 413인 `httperr.HTTPError` |
| 비어 있지 않은 본문의 Content-Type이 없거나 지원하지 않는 형식 | 상태가 415인 `httperr.HTTPError` |
| 빈 본문에 대한 `Bind` | Content-Type과 무관하게 성공하며 값을 바꾸지 않습니다. |
| 취소 또는 원래 스트림의 I/O 오류 | 원인을 보존하며 일반 입력 400으로 바꾸지 않습니다. |

HTTP 오류의 공개 메시지는 상태 설명이며 원인은 `errors.Is`와 `errors.As`로 확인합니다. multipart는 마지막 boundary 뒤의 데이터에도 전체 본문 한도를 적용합니다. 파싱에 실패한 요청의 임시 파일도 정리합니다.

읽기 전후와 디코더 실행 후에 취소를 검사합니다. 임의의 `io.Reader`에서 이미 막힌 읽기를 강제로 중단하는 기능은 제공하지 않습니다. 실제 소켓 읽기 종료와 시간 제한은 서버가 관리해야 합니다. 같은 `Body`를 여러 goroutine에서 동시에 사용하지 않습니다.

## 검증

저장소 루트에서 실행합니다.

```sh
GOWORK=off go run ./scripts/test test -mod=readonly -race -count=1 ./internal/input
```

테스트는 입력 형식, 출처 분리, null, 본문 재생 거부, 파일과 폼의 공유, 디스크 임시 파일 정리, 취소와 I/O 원인을 검사합니다. 실제 HTTP 연결에서는 파서를 읽기 전에 거부한 요청의 읽기 횟수와 chunked 본문의 크기 제한도 확인합니다. 이 검사는 새 공개 net/http 어댑터나 HTTP v3의 완료를 뜻하지 않습니다.
