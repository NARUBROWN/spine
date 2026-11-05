# Discord 태그 알림 Action

버전 태그를 Preview, RC, 정식 채널로 나누어 태그 생성 사실을 알립니다. 알림 제목에는 저장소 이름과 태그를 표시하고 태그 페이지와 Actions 실행 기록으로 연결합니다. 릴리스 검증과 배포 완료를 알리는 단계는 별도로 연결해야 합니다.

## 적용 대상

Spine, Spine CLI, Echo, Gorilla, Kafka, RabbitMQ, NATS, validator-playground와 문서 사이트에서 사용합니다. 테크블로그와 예제 프로젝트에는 적용하지 않습니다. 소비 저장소는 이 Action을 전체 커밋 SHA로 고정합니다. 코어 자체는 checkout한 로컬 Action을 사용합니다.

코어, CLI와 플랫폼은 기존 발행 workflow의 태그 생성 성공 직후 호출합니다. `GITHUB_TOKEN`이 만든 tag/release 이벤트에 후속 workflow를 기대하지 않습니다. 문서 사이트는 별도 발행 pipeline이 없어 새 버전 태그의 push 이벤트에서 호출합니다. 태그 생성과 Release 게시, 배포를 자동으로 시작하지 않습니다.

## 입력

| 입력 | 기본값 | 동작 |
| --- | --- | --- |
| `plan` | 빈 문자열 | 발행 계획 JSON 경로. 지정하면 `version`, `channel`, `repository`, `source_sha`를 사용합니다. 상대 경로는 호출 저장소 workspace 기준입니다. 생략하면 `GITHUB_REF_NAME`, `GITHUB_SHA`, `GITHUB_REPOSITORY`와 `GITHUB_REF_TYPE=tag`가 필요합니다. |
| `webhook_preview` | 빈 문자열 | `vX.Y.Z-preview.N` 태그의 webhook URL |
| `webhook_rc` | 빈 문자열 | `vX.Y.Z-rc.N` 태그의 webhook URL |
| `webhook_release` | 빈 문자열 | `vX.Y.Z` 태그의 webhook URL |

각 저장소의 **Settings → Secrets and variables → Actions**에 `DISCORD_WEBHOOK_PREVIEW`, `DISCORD_WEBHOOK_RC`, `DISCORD_WEBHOOK_RELEASE`를 repository secret으로 등록하고 해당 입력으로 전달합니다. `N`은 1 이상의 정수이며 알 수 없는 태그 형식과 계획의 채널 불일치는 거부합니다.

Ubuntu runner의 Python 3 표준 라이브러리만 사용합니다. 호출 저장소의 Python 패키지나 `release.py`에 의존하지 않습니다. webhook URL은 코드, step output과 로그로 출력하지 않습니다. 포럼과 미디어 채널은 URL에 대상 `thread_id`가 필요합니다.

## 전송 확인과 실패 처리

`wait=true`로 Discord의 메시지 생성 응답과 `id`를 확인하며 연결 timeout은 20초입니다. HTTP 오류, 응답 오류와 secret 누락은 경고를 출력하고 코드 1로 종료합니다. 호출 단계는 `continue-on-error: true`로 태그 이후의 발행 검증을 계속합니다.

자동 재시도는 하지 않습니다. 응답이 유실되면 실제 채널에서 수신 여부를 먼저 확인하고, 이미 생성한 태그를 다시 발행해 알림만 재시도하지 않습니다. `--dry-run`은 webhook 없이 payload를 출력하며 실행 환경 변수는 그대로 필요합니다.
