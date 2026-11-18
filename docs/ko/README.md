# 한국어 원문 모음

문서의 기본 언어는 영어입니다. 이 폴더에는 정리 전 한국어 원문을 보관합니다.
현재 `v0.7.6` 소스의 API와 실행 방법은 [영어 기본 문서](../README.md)를 기준으로 확인하세요.
원문에 나오는 과거 컨텍스트, 발행 API와 릴리스 상태는 현재 소스와 다를 수 있습니다.

## 프로젝트 소개

- [FOSS for All 로고 출처 및 권리 표기](attribution.md)
- [> 정리 전 한국어 README 원문입니다. 현재 소스 API는 [영어 기본 문서](../README.md)를 기준으로 확인하세요.](overview.md)
- [보안 설정 참고 자료](security-configuration.md)

## 예제

- [CLI 계약과 E2E를 공유하는 최소 예제](examples/demo.md)
- [Spine 플랫폼 참고 애플리케이션](examples/reference.md)

## 계약과 패키지

- [이전 계약 경로 호환성](reference/adapter-compatibility.md)
- [공통 실행 계약](reference/contracts.md)
- [이벤트 어댑터 계약](reference/event-platform.md)
- [요청 본문 파서](reference/http-inputs.md)
- [HTTP v3 계약](reference/http-platform.md)
- [임베드 OpenAPI 문서 UI](reference/openapi-ui.md)
- [런타임 OpenAPI](reference/openapi.md)
- [DTO 검증 공통 계약](reference/validation.md)
- [WebSocket 플랫폼 계약](reference/websocket-platform.md)

## 마이그레이션 기록

- [앱 수명의 DI Publisher로 이전](migration/app-publisher.md)
- [하나의 Constructor 호출에 역할별 생성자 등록](migration/component-contracts.md)
- [컨트롤러 안에서 HTTP 경로 등록하기](migration/controller-routes.md)
- [이벤트 플랫폼으로 전환하기](migration/event-platforms.md)
- [Echo 플랫폼으로 전환하기](migration/http-adapter-modules.md)
- [HTTP v3 입력 처리와 준비 API 전환](migration/http-v3.md)
- [경로, 쿼리와 헤더 구조체 입력으로 전환하기](migration/structured-inputs.md)
- [v0.5.1 마이그레이션](migration/v0.5.1.md)
- [v0.5.2 마이그레이션](migration/v0.5.2.md)
- [v0.5 보안 기본값 마이그레이션](migration/v0.5.md)
- [Spine v0.6.5 전환](migration/v0.6.5.md)
- [Spine v0.6.6 전환](migration/v0.6.6.md)
- [Spine v0.7.0 전환](migration/v0.7.0.md)
- [v0.7.1 긴급 패치](migration/v0.7.1.md)
- [v0.7.2 핫픽스 전환](migration/v0.7.2.md)
- [v0.7.5 프리뷰 API 전환](migration/v0.7.5.md)
- [Spine v0.7.6 마이그레이션](migration/v0.7.6.md)
- [WebSocket 구현을 독립 플랫폼으로 선택하기](migration/websocket-platforms.md)

## 테스트

- [Spine 테스트 실행](testing/README.md)
- [HTTP v3 두 엔진 적합성 검증](testing/http-contract.md)
- [HTTP 플랫폼 비교 E2E](testing/http-platforms.md)
- [OpenAPI와 실제 HTTP 비교](testing/openapi.md)
- [공개 플랫폼 계약 E2E](testing/platforms.md)

## 유지보수

- [저장소 작업 지침](maintenance/agent-instructions.md)
- [작업 규칙](maintenance/agent-rules.md)
- [Discord 태그 알림 Action](maintenance/discord-tag.md)
- [Spine 릴리스 운영](maintenance/releasing.md)
