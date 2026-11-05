# 요청 본문 파서

`Body`는 요청 하나의 JSON, XML, URL-encoded form과 multipart를 처리하는 내부 구현입니다. 본문만 읽으며 URL 쿼리와 경로 값을 폼에 합치지 않습니다. 폼 필드는 기존 `BodyFields`를 사용하고 JSON의 null 판정은 `pkg/jsonx`에 맡깁니다.

HTTP v3 실행은 이 파서를 요청별로 사용합니다. 플랫폼은 원시 reader와 읽기 한도를 제공하고, 커널은 실제 Bind 또는 MultipartForm 호출 때 파서를 생성합니다.

## 생성과 설정

```go
func NewBody(ctx context.Context, reader io.Reader, contentType string, options BodyOptions) (*Body, error)
```

생성 시에는 본문을 읽지 않습니다. `ctx`와 `reader`는 nil이 아니어야 합니다. 원래 스트림은 서버 또는 어댑터가 닫습니다.

| 설정 | 생략 시 값 | 허용 값과 의미 |
| --- | --- | --- |
| `MaxBytes` | `32 << 20` 바이트 | 양수는 전체 본문 한도, 음수는 제한 해제입니다. 읽을 때 한도를 적용하므로 Content-Length가 없는 요청도 검사합니다. |
| `MultipartMemory` | `32 << 20` 바이트 | 양수는 multipart 파일의 메모리 예산입니다. 초과 파일 데이터는 임시 파일에 저장하며 음수는 거부합니다. 전체 요청 크기 제한이나 전체 메모리 사용량의 상한은 아닙니다. |

실제 HTTP 실행은 `boot.HTTPOptions.MaxBodyBytes`를 플랫폼 reader에 적용하므로 이 파서의 MaxBytes는 -1로 둡니다. `boot.HTTPOptions.MultipartMemoryBytes`는 MultipartMemory로 전달하며 0은 32 MiB입니다. 파서 단독 사용의 MaxBytes 기본값은 유지합니다.

## 바인딩과 파일 수명

- `Bind(target)`은 본문을 한 번 바인딩합니다. 성공 후 다시 호출하면 `ErrBodyConsumed`를 반환하며, 실패 후 호출하면 저장한 오류를 반환합니다. 본문을 자동 재생하지 않습니다.
- JSON은 일반 값 타입의 명시적 null을 거부하고 포인터, map, slice와 사용자 `json.Unmarshaler`의 규칙을 유지합니다. XML은 표준 `xml.Decoder.Decode`로 값을 하나 읽습니다.
- 폼은 구조체 포인터의 `form` 필드에 매핑합니다. 반복 키는 슬라이스에 보존하고 쉼표를 구분자로 나누지 않습니다. 누락된 포인터는 nil을 유지하고 명시한 빈 숫자와 bool은 0과 false로 읽습니다. `encoding.TextUnmarshaler`도 호출합니다.
- multipart의 파일 필드는 `*multipart.FileHeader`, `[]*multipart.FileHeader`, `[]multipart.FileHeader`를 지원합니다. `MultipartForm()`으로 얻는 파일 목록과 폼 DTO가 같은 파싱 결과를 공유합니다. 폼과 파일 조회의 순서를 바꿔도 스트림을 다시 파싱하지 않으며 파싱 오류도 저장합니다.
- `Cleanup()`은 자신이 파싱한 임시 파일을 한 번 정리합니다. 반복 호출은 같은 정리 결과를 반환합니다. 정리 이후 파싱과 파일 조회는 `ErrBodyClosed`로 거부하며 원래 본문 스트림은 닫지 않습니다.

실행 경로는 인증 인터셉터가 요청을 허용한 뒤 바인딩하고, `AfterCompletion`까지 끝난 후 Execute 바깥에서 `Cleanup`을 호출합니다. 요청 뒤에도 파일이 필요하면 그 전에 별도 저장소로 복사해야 합니다. 통합 검사는 폼/파일 양순서, 컨트롤러 패닉, BeforeResponse 오류와 완료 훅 패닉 뒤의 파일 제거를 확인합니다.

Spine 입력은 Echo Binder/JSONSerializer, `UnmarshalParam`, `UnmarshalParams`와 `format` 태그를 호출하지 않습니다. 표준 JSON/XML 디코더와 `encoding.TextUnmarshaler`로 옮기거나 native Echo 경로를 사용하세요. Echo의 모든 확장과 동등한 바인딩을 보장하지 않습니다.

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
