# HTTP v3 두 엔진 적합성 검증

같은 `contract/http/v3`를 net/http와 fasthttp native 진입점에 연결해 실제 TCP 요청을 비교합니다. 코드는 검증용 최소 구현이며 제품 플랫폼이나 Spine pipeline이 아닙니다. fasthttp를 net/http handler로 변환하는 bridge는 사용하지 않습니다.

## 수정 검증 결과

2026-11-05 KST, Go 1.26.0 darwin/arm64에서 fasthttp v1.77.0의 EOF 처리를 분리 복사본에 패치해 검사했습니다. 같은 테스트 파일로 수정 전 54개 통과/2개 실패, 수정 후 56개 모두 통과했으며 생략은 없습니다. 기존 53개에 정상 chunked EOF와 연결 재사용 검사 3개를 추가했습니다.

| 검사 | 결과 |
| --- | --- |
| 순정 fasthttp를 사용하는 전체 race | 54개 통과, 기존 종료 오류 2개 실패 |
| EOF 패치를 적용한 전체 race | 56개 통과 |
| 기존 실패 2개를 각각 20회 반복 | 40개 통과 |
| 패치한 fasthttp의 stream/chunk/trailer 관련 race | 394개 통과 |
| 패치 조합 vet | 통과 |

패치는 `patches/fasthttp-v1.77.0-strict-chunk-eof.patch`입니다. 다음 두 위치에서 불완전한 종료의 `io.EOF`를 `io.ErrUnexpectedEOF`로 바꿉니다.

- 다음 chunk size를 읽기 전에 연결이 끝난 경우.
- 마지막 0 청크 뒤 trailer 영역이 끝나기 전에 연결이 끝난 경우.

fasthttp probe의 `StreamRequestBody: true`와 1바이트 초기 버퍼는 유지합니다. 계약 API와 기존 실패 검사의 기대값도 유지합니다. 정상 빈 본문, 정상 trailer와 반복 EOF 읽기 뒤 같은 연결 재사용을 함께 확인했습니다.

## 실행 방법

```sh
cd test/http-contract
python3 verify_strict_stream.py --output-dir /tmp/spine-http-eof-proof
```

검증기는 순정 버전의 두 실패를 확인한 뒤, 임시 디렉터리에 의존성과 probe를 복사하고 패치를 적용합니다. 해당 복사본의 `replace`만 바꿔 같은 테스트를 실행하며 원래 모듈 캐시, 코어 계약과 go.mod를 해시로 대조합니다. 로그와 `summary.json`은 지정한 출력 경로에 저장하고 임시 복사본은 정리합니다.

이 결과는 로컬 패치 조합의 검증입니다. 배포된 순정 v1.77.0은 아래 실패를 계속 재현합니다. 제품 플랫폼에 사용하려면 수정된 의존성의 제공 방식과 버전을 별도로 정해야 합니다.

## 순정 버전 기준 결과

| 실행 방식 | 기존 엔진별 검사 | 판정 |
| --- | --- | --- |
| net/http | 17개 통과 | 검사한 범위에서 통과 |
| fasthttp 스트리밍 | 15개 통과, 2개 실패 | chunked 종료를 정상 EOF와 구분하지 못함 |
| fasthttp 버퍼링 | 17개 통과 | 검사한 범위에서 통과, 메모리 정책 별도 필요 |

native 옵션 거부와 net/http 업그레이드 검사 2개는 엔진별 개수와 별도입니다. 위 표는 추가 연결 재사용 검사 전의 53개 기준입니다. 순정 버전으로 아래 명령을 실행하면 종료 오류 2개가 계속 실패합니다.

```sh
cd test/http-contract
GOWORK=off go test -race -count=1 -timeout=90s ./...
```

통과한 두 실행 방식만 재검사하려면 다음 필터를 사용합니다. 이 명령의 통과는 스트리밍 실패를 해결했다는 뜻이 아닙니다.

```sh
GOWORK=off go test -race -count=1 -run '/^(nethttp|fasthttp-buffered)$' -timeout=90s ./...
```

## 발견한 입력 오류 차이

고정 `Content-Length`보다 적은 바이트를 보낸 뒤 연결의 쓰기 방향을 닫으면 fasthttp 요청 스트림은 정상 EOF를 반환할 수 있습니다. 검증용 어댑터는 알려진 길이와 실제 읽은 길이를 비교해 이를 `io.ErrUnexpectedEOF`로 보완했습니다. reader getter의 무읽기와 본문 한도는 유지합니다.

길이를 모르는 chunked 입력에는 같은 보완을 적용할 수 없습니다. 다음 두 본문을 보낸 뒤 쓰기 방향을 닫으면 스트리밍 reader가 `ab`와 nil 오류를 반환합니다.

| 전송 본문 | 누락된 부분 |
| --- | --- |
| `2\r\nab\r\n` | 마지막 0 청크 |
| `2\r\nab\r\n0\r\n` | trailer 영역을 끝내는 빈 줄 |

net/http는 두 입력 모두 읽기 오류로 전달합니다. fasthttp 버퍼링 모드는 Execute 진입 전에 400 또는 연결 종료로 거부합니다. native 프로토콜 단계의 거부와 Execute에서 관측한 오류는 검사 결과에 구분해 남깁니다. 취소나 I/O 오류를 임의로 HTTP 400인 `RequestError`로 바꾸지 않습니다.

재현 명령은 현재 실패해야 합니다.

```sh
GOWORK=off go test -race -count=3 \
  -run 'TestTruncatedRequestPreservesReadFailure/^fasthttp$' \
  -timeout=30s ./...
```

이 문제는 일반 body reader의 정상 EOF만 보고 복원할 수 없습니다. 이번 보완은 native parser가 불완전한 종료를 오류로 드러내도록 한 로컬 패치입니다. 버퍼링 대조는 가능한 다른 구현 방식의 증거이며 제품의 메모리/전체 한도 정책을 채택한 것은 아닙니다. 버퍼링 probe의 큰 native 한도와 스트리밍 probe의 1바이트 초기 버퍼는 테스트 설정입니다.

소스 대조에서는 요청 스트림의 고정 길이 검사와 chunk 종료 처리를 확인했습니다. [fasthttp 스트림 소스](https://github.com/valyala/fasthttp/blob/v1.77.0/streaming.go), [청크 파서](https://github.com/valyala/fasthttp/blob/v1.77.0/http.go#L3382)

## 확인한 범위

- 실제 요청의 원시 경로, 원시 쿼리, 반복 헤더와 Host 포트.
- 헤더 맵 격리, native 요청 재사용 뒤 문자열/context 값 보존과 요청 종료 취소.
- JSON 출력 bytes의 반환 후 재사용, `Set-Cookie` 반복 값.
- 고정 길이와 chunked 본문의 정확한 한도/1바이트 초과, 음수 제한 해제와 반복 getter의 같은 읽기 위치.
- 잘못된 multipart를 거부하는 Execute 경로의 Body.Read 0회. 이는 Spine interceptor나 DTO 파서의 통합 검사가 아닙니다.
- 새 요청 503, native 경로/후처리/진행 Execute를 포함한 외부 서버 종료 대기와 시간 초과 뒤 재대기.
- 관리 서버의 정상 종료 정규화, 리스너 소유권, 중복 실행/재실행 거부와 동시 Shutdown.
- net/http 연결 해제 취소와 fasthttp의 해당 기능 미지원 선언.
- net/http raw 101 업그레이드 진입 추적과 fasthttp/기존 net/http WS 조합 거부. 전체 WebSocket 프로토콜은 검사하지 않습니다.
- 32개 동시 요청의 값과 본문 격리, 명시적 미지원 native WriteTimeout 거부.

## 아직 확인하지 않은 범위

이 모듈은 최소 어댑터 검증만 수행합니다. 실제 Spine v3/Echo 연결, DTO/validator/staging/완료 훅과 multipart 정리는 별도의 `scripts/verify-http-v3.py`로 검사합니다. 전체 서버 옵션의 실제 시간/크기 경계, TLS/HTTP2와 모든 잘못된 HTTP 프레이밍, 독립 net/http 제품과 공개 설치 조합은 남아 있습니다.

fasthttp 의존성과 Go 1.26 요구는 이 테스트 모듈에만 둡니다. 코어의 Go 1.25.5 설정과 의존성은 변경하지 않습니다. 사용 버전의 Go 요구는 [fasthttp go.mod](https://github.com/valyala/fasthttp/blob/v1.77.0/go.mod)에서 확인할 수 있습니다.
