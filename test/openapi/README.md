# OpenAPI와 실제 HTTP 비교

코어의 외부 의존성을 늘리지 않고 실제 Echo 바인딩과 JSON Schema 판정을 대조합니다. Go 1.26 이상이 필요합니다.

```sh
GOWORK=off go test -race ./... -count=1
```

별도 입력 래퍼나 응답 선언 없이 기존 DTO와 반환형을 사용합니다. JSON DTO의 35개 경계 입력, 기존 쿼리 DTO와 header.Values의 바인딩, 폼과 파일 업로드, nullable 응답, 문자열과 바이너리, 오류 응답을 검사합니다. 런타임에서 결정하는 상태와 바이너리 미디어 타입은 고정값으로 추측하지 않는지도 확인합니다. 모든 사용자 정의 검증이나 플랫폼의 판정 일치를 증명하지 않습니다.

`testdata/openapi-3.1-schema.json`은 [OpenAPI 공식 구조 스키마](https://spec.openapis.org/oas/3.1/schema/2022-10-07)를 보관한 파일입니다. 해당 파일에는 `testdata/OPENAPI-LICENSE.txt`의 Apache 2.0 라이선스를 적용합니다. 각 DTO의 유효성은 별도 JSON Schema 검사로 대조합니다.

validator는 설명 계약을 제공하는 공개 개발 커밋에 고정했습니다. 이 의존성은 아직 정식 릴리스 조합을 뜻하지 않습니다. 코어는 replace ../..로 현재 소스를 검사합니다.
