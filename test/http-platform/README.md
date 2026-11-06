# HTTP 플랫폼 비교 E2E

같은 Spine 컨트롤러와 DTO를 Echo, net/http에 연결하고 실제 TCP 요청으로 결과를 비교합니다. 관리 서버는 각 플랫폼의 `HTTPAdapter.Serve`로 실행하며 외부 서버의 Prepared 종료도 별도로 검사합니다.

```sh
python3 scripts/verify-http-platforms.py \
  --nethttp-root ../spine-nethttp-platform \
  --echo-root ../spine-echo-issue-28 \
  --output-dir /tmp/spine-http-platform-proof
```

명령은 코어 저장소 루트에서 실행합니다. 로컬 후보 조합은 코어 `v0.7.0-preview.3`, Echo `v0.2.0-preview.2`, net/http `v0.1.0-preview.1`이며 아직 발행하지 않았습니다. 스크립트는 임시 modfile의 replace로 세 작업트리를 연결하고 Go 1.25.5에서 race와 vet을 실행합니다. 제품 및 테스트 모듈의 원래 manifest는 변경하지 않습니다.

검사 범위는 다음과 같습니다.

- JSON, XML, 폼, 경로, 반복 쿼리와 Host 헤더의 DTO 결과, 오류 응답, 컨트롤러 및 validator 호출 횟수 비교
- 전역 인증 및 route 인증 거부 시 공통 파서 읽기 0회
- Content-Length와 chunked 본문의 JSON 및 multipart 크기 제한
- 폼과 파일의 인자 순서, 완료 훅까지의 파일 수명, 성공과 패닉 및 훅 오류 뒤 정리
- native 후처리 중 Close 대기 기한, 새 진입 503, 진행 요청 보존과 재대기
- WebSocket 업그레이드, 메시지와 앱 종료, runtime OpenAPI GET과 HEAD
- 준비 기본값과 실행 옵션의 HTTPS, TLS 설정 교체와 복사, 두 인증서 선택 콜백, 클라이언트 신뢰 검증 실패
- TLS의 인증 전 무읽기, chunked multipart 한도, 파일 정리와 native 후처리 drain, 클라이언트 연결 해제 취소, RunContext의 HTTPS OpenAPI

JSON 응답은 파싱 후 비교하므로 serializer의 공백과 줄바꿈은 비교하지 않습니다. TLS 검사는 HTTP/1.1로 실행하며 HTTP/2, 브로커, 운영 부하와 공개 모듈 설치는 별도 검증입니다.
