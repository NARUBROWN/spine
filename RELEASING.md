# Spine 릴리스 운영

코어는 v0.7.0, 기존 다섯 플랫폼은 v0.2.0, net/http와 NATS는 v0.1.0을 대상으로 Preview, RC, 정식 채널을 발행합니다. 코어 후보마다 공개된 플랫폼 태그를 하나의 검증 조합으로 고정합니다. 플랫폼 채널과 버전은 독립적입니다. 패키지 버전 번호는 독립적입니다.

모든 단계에서 태그를 발행하며 GitHub Release는 정식에서만 만듭니다. 예제 앱은 정식 태그를 사용하고 정식 발행 후 갱신합니다. 예제나 CLI 생성 레시피 갱신을 코어 후보 발행의 선행 조건으로 사용하지 않습니다.

## 정책과 소스 CI

`.github/release-policy.json`의 `channels`에 각 채널의 코어와 일곱 플랫폼 태그를 고정합니다. 현재 Preview 코어 후보와 플랫폼 검증 대상은 다음과 같습니다.

| 모듈 | Preview 버전 |
| --- | --- |
| Spine 코어 | `v0.7.0-preview.13` |
| Echo | `v0.2.0-preview.4` |
| Gorilla | `v0.2.0-preview.2` |
| Kafka | `v0.2.0-preview.2` |
| RabbitMQ | `v0.2.0-preview.2` |
| playground validator | `v0.2.0-preview.6` |
| net/http | `v0.1.0-preview.2` |
| NATS | `v0.1.0-preview.1` |

코어 `v0.7.0-preview.13`은 새 후보이며 플랫폼 태그는 기존 공개 버전입니다. 표의 태그가 있다는 사실만으로 호환성이 확인되는 것은 아닙니다. 코어가 고정한 공개 모듈을 함께 설치하고 전체 조합 검증을 통과해야 합니다. RC와 정식 조합도 독립된 플랫폼 태그를 지정하며 현재 정책은 위 태그를 재사용합니다. 정식 발행 전에는 Preview 플랫폼 태그를 그대로 사용하는 것이 적절한지 검토합니다.

HTTP v3의 net/http 플랫폼을 독립 저장소와 여섯 번째 발행 모듈로 편입합니다. 소스 CI는 고정한 net/http 커밋을 사용하고 HTTP/TLS 비교 E2E를 실행합니다. 공개 조합 완료에는 고정한 net/http 태그의 설치와 조합 검증이 필요합니다. 원격 저장소 준비와 PR 게시는 후보 태그 발행을 뜻하지 않습니다.

새 코어 태그가 아직 없을 때 소스 CI는 `ci_sources`의 전체 커밋 SHA와 현재 코어를 임시 go.work로 연결합니다. 버전별 임시 replace는 미발행 태그의 메타데이터 조회도 로컬 소스로 연결하며 저장소의 go.mod에는 넣지 않습니다. 이 결과를 공개 설치 성공으로 기록하지 않습니다. 플랫폼 소스 CI도 `ci_core`의 전체 커밋 SHA를 사용합니다.

태그 생성 전에는 공개 SHA를 조회해 소비자 빌드를 검사하고, 생성 후에는 태그로 다시 검사합니다. 플랫폼은 자체 일정에 따라 발행하며, 코어는 공개된 플랫폼 태그를 선택해 조합을 검증합니다. 공개 검증은 새로운 모듈 캐시, `GOWORK=off`, go.mod의 로컬 replace가 없는 별도 소비자에서 수행합니다. 정책과 실제 선택된 버전이 다르면 실패합니다.

## 코어 후보와 전체 조합 완료

코어의 go.mod는 플랫폼에 의존하지 않습니다. 코어가 플랫폼의 완성 태그를 기다리는 순환을 피하도록 다음 두 단계를 분리합니다.

1. **코어 후보 태그 발행:** 코어 자체의 race, vet, 정책과 발행 코드 검사와 공개 SHA로 소비자 빌드를 마친 뒤 태그를 생성합니다. 생성한 태그로 공개 코어를 다시 빌드하고 `core-candidate-<태그>` artifact를 남깁니다. 이 상태는 아직 승격 가능한 후보가 아닙니다.
2. **전체 조합 완료:** 일곱 플랫폼 태그의 정확한 커밋을 기록하고, 플랫폼의 과거 코어 의존 버전과 관계없이 선택한 코어와 공개 모듈을 함께 설치해 검증합니다. 전체 CI, HTTP, WebSocket, 실제 Kafka, RabbitMQ와 NATS JetStream, validator와 OpenAPI 검증을 마친 뒤 `release-manifest-<태그>` artifact를 남깁니다. 이 기록이 있어야 다음 채널로 승격할 수 있습니다. manifest의 `validation.scope` 값 `cohort`는 이 전체 릴리스 조합 검증을 뜻하는 고정 식별자입니다.

실행 순서는 **코어 후보 → 공개된 플랫폼 태그 선택 → 코어 전체 조합 완료**입니다. 플랫폼 자체 변경이 필요한 경우에만 별도로 새 플랫폼 태그를 발행합니다. Preview, RC, 정식 모두 같은 순서입니다. 코어의 정식 GitHub Release는 전체 조합 완료 후에만 생성합니다. 각 플랫폼의 정식 GitHub Release는 해당 플랫폼의 공개 검증 후 생성합니다.

태그는 생성되면 Go 모듈로 조회될 수 있습니다. 여러 저장소의 태그가 원자적으로 생성되는 것은 아닙니다. 실패한 태그를 이동하거나 덮어쓰지 않고 다음 후보 번호를 사용합니다.

## 최초 채널 설정

하위 저장소의 기존 정식 태그와 main을 이동하지 않습니다. 먼저 다음 명령으로 preview와 rc 생성 계획을 확인합니다.

```sh
python3 scripts/release/configure_branches.py --bootstrap
```

검토한 설정을 적용할 때만 `--bootstrap --apply`를 사용합니다. 누락된 preview와 rc를 현재 main에서 만들며 이미 존재하는 브랜치는 변경하지 않습니다. 첫 작업 PR은 preview를 대상으로 합니다.

채널 코드가 preview에 반영된 후 `configure_branches.py`로 보호 규칙을 조회하고, 검토한 경우에만 `--apply`를 사용합니다. 이 명령은 기본 브랜치를 preview로 설정하고 세 브랜치의 PR, 대상별 `Verified Source / <대상>`과 `Branch Flow / <대상>` 검사, merge 방식, 버전 태그 수정과 삭제 금지, 발행 환경을 적용합니다. 정식 환경은 지정 리뷰어, 관리자 우회 금지와 main 제한을 요구합니다. 이전 main 보호를 제거해야 할 때는 먼저 새 보호를 적용하고 기존 설정을 로컬 Git 디렉터리에 백업합니다.

이 설정은 GitHub 상태를 변경합니다. 저장소와 적용 계획을 검토한 후 실행하세요. 정식 환경 승인은 사용자가 직접 처리합니다.

## Preview 발행

작업 PR을 preview에 반영하고 소스 CI를 통과시킵니다. 코어 저장소의 **Actions → Publish Preview**에서 preview 브랜치, 새 버전과 preview HEAD의 전체 SHA를 지정합니다.

코어 후보 artifact가 생성되면 정책에 기록된 기존 플랫폼 태그와 커밋을 확인하고 **Complete Core Release Combination**에서 코어 태그를 지정합니다. 플랫폼 manifest에 기록된 과거 코어 버전은 조합 완료의 조건이 아닙니다. 코어 저장소는 고정된 태그를 공개 모듈로 설치해 HTTP, WebSocket, validator와 실제 브로커 조합을 검증하고 그 결과와 태그 커밋을 manifest에 남깁니다.

코어 채널 HEAD는 후보 발행 시점과 같아야 합니다. 문서와 버전 정책도 후보 발행 전에 확정하세요. 태그가 없거나 바뀌거나 공개 테스트가 실패하거나 생략되면 완료하지 않습니다.

## RC와 정식 승격

Preview 전체 조합 완료 후 72시간이 지나야 RC를 발행할 수 있습니다. RC 전체 조합 완료 후 24시간, 기존 정식 발행 후 168시간이 지나야 다음 정식을 발행할 수 있습니다. 긴급 관찰 기간 단축은 20자 이상의 사유를 기록하며 CI, 공개 검증과 정식 승인을 생략하지 않습니다.

채널별 의존 버전 변경은 전용 승격 브랜치에서 준비합니다. RC 예시는 다음과 같습니다.

```sh
git switch -c promote/rc/v0.7.0-rc.1 v0.7.0-preview.13
python3 scripts/release/pin_channel.py --channel rc
python3 scripts/release/pin_channel.py --channel rc --apply
```

이 브랜치의 PR 대상은 rc입니다. 아직 발행되지 않은 코어 RC 태그를 조회하지 않도록 이 단계에서 `--tidy`를 사용하지 않습니다. 코어 RC 후보 태그를 발행한 후 정책에 고정한 기존 플랫폼 태그로 전체 조합을 다시 검증합니다. 플랫폼이 변경되지 않았다면 별도 플랫폼 승격 PR이나 태그는 필요하지 않습니다.

정식은 코어 `promote/stable/v0.7.0` 브랜치에서 stable 핀을 준비하고 main을 대상으로 PR을 만듭니다. 플랫폼은 선택한 기존 태그를 재사용하거나 독립적으로 발행합니다. 일반 작업 PR은 preview만 대상으로 합니다. 승격 PR은 merge commit으로 병합합니다.

발행 코드는 이전 후보가 승격 PR의 조상인지, 병합된 PR의 방향과 두 번째 부모가 맞는지 확인합니다. 소스 fingerprint는 공식 모듈 require의 같은 기본 버전 내 채널 태그, 해당 go.sum 항목과 CI 출처만 정규화합니다. 코드, 일반 의존성, replace, 기본 의존 버전과 관찰 정책의 변경은 새 Preview가 필요합니다. 실제 채널 핀과 공개 모듈 조합은 매 단계 다시 검증합니다.

RC 발행 입력은 새 RC 버전과 완료된 Preview 태그입니다. 정식 발행 입력은 완료된 RC 태그이며 정식 버전은 자동 결정합니다. 코어 후보 발행과 전체 조합 완료의 정식 환경 승인 요청은 각각 사용자가 직접 처리합니다.

## Discord 태그 알림

Publish Preview, Publish Rc, Publish Stable에서 태그 생성에 성공하면 바로 Discord 알림을 보냅니다. 버전 태그로 채널을 선택합니다.

| 태그 형식 | GitHub Actions repository secret |
| --- | --- |
| `vX.Y.Z-preview.N` | `DISCORD_WEBHOOK_PREVIEW` |
| `vX.Y.Z-rc.N` | `DISCORD_WEBHOOK_RC` |
| `vX.Y.Z` | `DISCORD_WEBHOOK_RELEASE` |

각 Discord 텍스트 채널의 **채널 편집 → 연동 → 웹후크**에서 URL을 복사하고, 저장소의 **Settings → Secrets and variables → Actions**에 해당 이름의 repository secret으로 등록합니다. URL은 코드나 로그에 기록하지 않습니다. 포럼과 미디어 채널은 webhook URL에 대상 `thread_id`가 필요합니다.

이 알림은 태그 생성 사실과 발행 workflow 링크를 표시합니다. 정식 태그 알림도 전체 조합 완료나 GitHub Release 게시를 뜻하지 않습니다. 태그 생성 시점에는 릴리스 노트가 없으므로 태그 페이지로 연결합니다. GitHub Release 이벤트와 수동 태그 push는 별도 알림 대상으로 연결하지 않습니다.

`GITHUB_TOKEN`으로 만든 태그는 다른 workflow를 자동 실행하지 않으므로, 재사용 발행 workflow의 태그 생성 단계 다음에서 [공통 Discord Action](.github/actions/discord-tag/README.md)을 직접 호출합니다. 호출 workflow는 위 세 secret만 이름으로 전달합니다. 알림 단계는 webhook을 step output으로 내보내지 않습니다. 알림 제목에는 저장소 이름을 넣어 같은 Discord 채널에서도 프로젝트를 구분합니다.

전송은 `wait=true`로 메시지 생성 응답을 확인합니다. secret 누락과 HTTP 오류는 해당 단계에 경고를 남기며 태그 이후 공개 검증을 계속합니다. 전송을 자동 재시도하지 않으며, 응답 유실 시 실제 채널을 확인해야 합니다. 이미 생성된 태그를 다시 발행해 알림만 재시도하지 않습니다.

저장한 발행 계획으로 전송할 payload를 확인할 수 있습니다. 아래 실행자와 실행 ID는 실제 발행 값으로 바꿉니다. webhook secret은 필요하지 않습니다.

```sh
GITHUB_REPOSITORY=NARUBROWN/spine \
GITHUB_ACTOR=NARUBROWN \
GITHUB_RUN_ID=123456789 \
python3 .github/actions/discord-tag/notify.py --plan release-plan.json --dry-run
```

## 검증 증거

Preview와 RC는 GitHub 사전 릴리스를 만들지 않습니다. artifact는 90일 보관을 요청합니다. 승격 시 다음을 확인합니다.

- 태그에 대응하는 단일 manifest, artifact 만료 여부와 ZIP SHA-256
- 저장소, 채널, 소스 SHA와 트리, 정규화한 소스 fingerprint
- 성공한 발행 workflow의 경로, 브랜치, HEAD, 실행 ID와 재실행 번호
- 공개 검증 범위가 코어 단독인지, 플랫폼인지, 전체 조합인지
- 플랫폼이 사용한 코어의 정확한 태그와 고정한 채널별 모듈 조합

기록이 없거나 바뀌면 승격하지 않습니다. v1 정책의 옛 후보를 v2 조합으로 자동 승격하지 않습니다.

## 정식 발행 후 예제 검증

예제 저장소의 go.mod를 완료된 정식 조합으로 갱신하고 커밋을 공개합니다. 정책의 `post_release_examples`를 그 커밋으로 바꾼 뒤 main에서 **Verify Examples After Stable Release**를 실행합니다.

이 workflow는 완료된 정식 manifest와 예제의 명시적인 버전 고정을 확인하고 실제 WebSocket과 브로커 테스트를 실행합니다. 예제 버전을 검증 과정에서 자동으로 덮어써서 성공으로 처리하지 않습니다. 이 결과는 별도 artifact이며 이미 발행한 태그나 manifest를 변경하지 않습니다.

## 로컬 검증

```sh
python3 -m unittest discover -s scripts/release -p 'test_*.py'
node --test scripts/release/branch_flow.test.cjs
python3 scripts/release/release.py validate-policy
python3 scripts/release/release.py check-module --channel preview
actionlint -shellcheck= -pyflakes=
```

미발행 태그가 있는 소스 검증은 임시 workspace를 사용합니다. 독립 공개 설치 검증은 태그를 실제 발행한 뒤 workflow에서 수행해야 합니다. 로컬 검증을 GitHub CI나 태그 발행 성공으로 보고하지 않습니다.
