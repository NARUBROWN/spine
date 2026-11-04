# OpenAPI와 실제 HTTP 비교

코어의 외부 의존성을 늘리지 않고 실제 Echo 바인딩과 JSON Schema 판정을 대조합니다. Go 1.26 이상이 필요합니다.

현재 작업 소스는 함께 수정한 코어, Echo 플랫폼과 validator를 포함하는 임시 go.work로 검증합니다. 기존 공개 의존성만으로는 최신 출처별 검증과 바인딩 변경을 검사할 수 없습니다. 공개 버전 고정을 갱신한 뒤 다음 명령으로 독립 조합을 검증해야 합니다.

```sh
GOWORK=off go test -race ./... -count=1
```

별도 입력 래퍼나 응답 선언 없이 기존 DTO와 반환형을 사용합니다. JSON DTO의 35개 경계 입력, 별도 쿼리와 헤더 구조체의 바인딩과 검증, 본문과의 분리, 폼과 파일 업로드, nullable 응답, 문자열과 바이너리, 오류 응답을 검사합니다. 런타임에서 결정하는 상태와 바이너리 미디어 타입은 고정값으로 추측하지 않는지도 확인합니다. 모든 사용자 정의 검증이나 플랫폼의 판정 일치를 증명하지 않습니다.

`testdata/openapi-3.1-schema.json`은 [OpenAPI 공식 구조 스키마](https://spec.openapis.org/oas/3.1/schema/2022-10-07)를 보관한 파일입니다. 해당 파일에는 `testdata/OPENAPI-LICENSE.txt`의 Apache 2.0 라이선스를 적용합니다. 각 DTO의 유효성은 별도 JSON Schema 검사로 대조합니다.

경로 검사는 다섯 개의 변수를 구조체 하나로 받는 실제 HTTP 요청, 이름 기반 매핑, 포인터와 0 및 false, 출처별 오류, 사용자 TextUnmarshaler, 미사용 경로 키와 기존 래퍼 전환을 다룹니다.

Deprecated query.Values와 header.Values는 실제 HTTP 요청에서 기존 조회와 변환 기본값, 반복 값, 빈 헤더와 Host를 확인합니다. Deprecated query.Pagination의 기존 기본값과 page, size 명세도 확인합니다. context.Context와 컨트롤러 컨텍스트의 주입도 함께 확인하며 동적 입력 키는 명세에서 추측하지 않는지 검사합니다. Pagination을 포함한 기존 API와 새 출처 DTO의 혼용은 별도의 코어 부팅 검사로 거부합니다.

회귀 검사는 폼 본문과 쿼리의 출처 분리, 임베딩된 폼 필드, 동명 패키지 타입을 담은 익명 응답, 배열과 byte slice의 요청 및 응답 표현, 제외한 입력과 필수 검증의 충돌도 다룹니다. 코어는 replace ../..로 현재 소스를 검사합니다. go.mod의 Echo와 validator 고정 버전에는 아직 최신 로컬 변경이 포함되지 않았습니다.
