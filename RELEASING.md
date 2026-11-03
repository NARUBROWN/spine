# Spine 릴리스 운영

브랜치 흐름은 **작업 브랜치 → `preview` → `rc` → `main`**입니다. 일반 작업 PR은 `preview`를 대상으로 합니다. 브랜치에 병합하는 작업과 버전 태그를 발행하는 작업은 분리합니다. 태그 발행은 GitHub Actions의 **Run workflow**로 실행합니다.

모든 단계에서 태그를 발행하고, GitHub Release는 Production(Stable)에서만 만듭니다. Preview와 RC는 태그와 Actions 검증 기록을 사용하며 GitHub 사전 릴리스는 만들지 않습니다.

## PR 방향과 브랜치 보호

| PR 출발 브랜치 | 허용 대상 |
| --- | --- |
| 일반 작업 브랜치, 외부 fork | `preview` |
| 같은 저장소의 `preview` | `rc` |
| 같은 저장소의 `rc` | `main` |
| 같은 저장소의 `main` | 없음 |

잘못된 방향의 PR은 `Branch Flow / <대상>` 필수 상태를 실패로 표시하고 자동으로 닫습니다. 대상을 바꾼 뒤 다시 열 수 있습니다. 다른 fork의 `preview`, `rc`라는 이름으로 승격 권한을 얻을 수는 없습니다.

세 채널 브랜치는 직접 push, 강제 push, 삭제를 금지하고 PR, 대상별 `Verified Source / <대상>`과 `Branch Flow / <대상>` 검사를 요구합니다. 같은 커밋으로 다른 대상을 향한 PR을 열어도 필수 상태를 공유하지 않습니다. 관리자도 우회할 수 없습니다. 현재 단일 관리자가 자신의 PR을 처리할 수 있도록 승인 리뷰 개수는 0으로 두고, CI와 방향 검사로 병합을 보호합니다. 정식 발행의 별도 사용자 승인은 그대로 유지합니다.

GitHub 설정은 [configure_branches.py](scripts/release/configure_branches.py)로 재현합니다. `python3 scripts/release/configure_branches.py`는 현재 설정을 읽어 검사하고, `--apply`를 붙이면 채널 ruleset, 기본 브랜치, Preview/RC 실행 환경을 적용합니다. 적용 전 `preview`에 PR 방향 workflow가 있어야 하며, 기존 main 보호 설정은 `.git/branch-flow-setup/`에 백업합니다. 정식 발행 승인 환경은 이 명령으로 변경하지 않습니다.

`rc`와 `main` 승격은 merge commit만 사용합니다. squash, rebase 승격은 허용하지 않습니다. RC에서 수정이 필요해도 작업 PR을 RC로 바로 넣지 않고, 수정 브랜치 → `preview` → `rc` 순서로 다시 검증합니다.

승격 merge 때문에 SHA가 달라질 수 있습니다. 릴리스 코드는 병합된 PR의 출발, 대상, merge commit의 두 번째 부모, 이전 후보와 소스 트리의 동일성을 확인하고 새 SHA에서 전체 CI, 공개 설치 검증을 다시 수행합니다. 승인 대기 중 채널 HEAD가 바뀌면 발행을 중단합니다.

| 단계 | 버전 태그 | 조건 | GitHub Release |
| --- | --- | --- | --- |
| Preview | `v0.6.6-preview.1` | `preview` HEAD, 전체 CI와 공개 설치 검증 통과 | 발행하지 않음 |
| RC | `v0.6.6-rc.1` | `preview → rc` 병합, 성공한 Preview, 최소 72시간 | 발행하지 않음 |
| Production(Stable) | `v0.6.6` | `rc → main` 병합, 성공한 RC 24시간, 정식 간격 168시간, 지정 리뷰어 승인 | 정식 발행 |

기본 정책과 검증할 CLI, 플랫폼, 예제 버전은 [release-policy.json](.github/release-policy.json)에 기록합니다. 한 번 발행한 태그는 이동하거나 재사용하지 않습니다. 같은 채널에서는 기존보다 큰 번호를 사용합니다.

## 출시 전 준비

1. `next_version`, 플랫폼 버전과 `cli_version`, 예제의 전체 커밋 SHA를 정책 파일에서 확인합니다. 변경하지 않은 모듈은 기존 버전을 재사용합니다.
2. 공개 API나 import 변경을 `docs/migration/<버전>.md`에 기록합니다. Preview에서 사용자가 미리 대응할 수 있도록 변경 내용과 전환 안내를 제공합니다.
3. CLI 생성 레시피는 기본 버전 그대로 검사한 뒤 후보 코어를 명시적으로 고정해 호환성을 검사합니다. 보고서에는 설치한 CLI 버전과 실제 선택된 코어, 플랫폼 버전이 남습니다. 기존 CLI의 기본 생성 버전이 후보 버전으로 바뀌었다고 해석하지 않습니다.
4. 하위 예제도 먼저 커밋해 공개하고 정책 파일의 `examples.*.ref`를 갱신합니다. 이 저장소의 미커밋 파일이나 개발자의 로컬 replace로 공개 검증을 대신하지 않습니다.

현재 정책에 기록된 chat `7379abc`와 MSA `c849986`은 Spine v0.6.0 기반 공개 커밋입니다. v0.6.5 이상으로 이식한 예제 커밋을 공개해 참조를 갱신하기 전에는 Preview 발행이 중단됩니다. 기존 참조를 자동으로 고쳐 검증 성공으로 처리하지 않습니다.

## Preview 발행

GitHub 저장소의 **Actions → Publish Preview → Run workflow**에서 브랜치는 `preview`를 선택합니다.

- `version`: `v0.6.6-preview.1`
- `source_sha`: `preview` HEAD의 전체 40자리 커밋 SHA

workflow는 다음 순서로 실행합니다.

1. 버전 형식, 대상 버전, `preview` HEAD 일치, 태그 미존재를 확인합니다.
2. 해당 SHA를 checkout한 기존 전체 CI와 공개 설치 검증을 실행합니다.
3. 두 검사가 성공하면 태그를 생성합니다.
4. 새 태그를 Go 모듈로 다시 설치해 같은 공개 검증을 실행합니다.
5. 성공하면 `release-manifest-<태그>` Actions artifact에 `release-manifest.json`을 저장합니다. GitHub Release는 만들지 않습니다.

Preview 수정은 새 커밋과 새 번호로 발행합니다. preview의 최신 커밋을 기존 Preview 태그에 덮어쓰지 않습니다.

## RC 발행

먼저 `preview → rc` PR을 merge commit으로 병합합니다. **Actions → Publish RC → Run workflow**에서 `rc`를 선택합니다.

- `version`: `v0.6.6-rc.1`
- `preview_tag`: 공개 검증을 마친 `v0.6.6-preview.1`
- `observation_override_reason`: 평소에는 비웁니다.

Preview 태그의 공개 설치 검증이 끝나고 manifest artifact를 저장한 시점부터 72시간이 지나야 합니다. 태그, manifest와 발행 workflow의 성공 여부를 대조하며, Preview와 동일한 소스 트리를 갖는 승격 merge commit과 호환 버전 조합을 다시 검증합니다. RC도 태그와 Actions 검증 기록만 남깁니다.

승격 검사는 태그별 manifest artifact가 하나인지 확인하고 artifact ID로 내려받아 SHA-256을 검증합니다. 저장소, 버전, 브랜치, 소스 SHA와 트리, 실행 ID와 재실행 번호, 호환 버전 조합과 공개 검증 결과가 일치해야 합니다. 발행 workflow 전체가 성공해야 승격할 수 있으며, 출처 artifact의 ID와 해시도 승인 계획에 고정합니다.

검증 artifact는 90일 보관을 요청합니다. 저장소의 보관 정책 안에서 사용해야 하며, 기록이 삭제되거나 만료되면 해당 후보의 승격을 중단하고 새 Preview부터 검증합니다. [GitHub Actions artifact 보관](https://github.com/actions/upload-artifact#retention-period)

RC에서 수정이 필요하면 변경을 작업 브랜치에서 `preview`에 반영하고 새 Preview를 검증한 뒤 새 RC를 발행합니다. API 변경을 RC에 몰래 추가하지 않습니다. 긴급 수정은 아래 관찰 기간 단축 규칙을 사용할 수 있습니다.

## Stable 승격

먼저 `rc → main` PR을 merge commit으로 병합합니다. **Actions → Publish Stable → Run workflow**에서 `main`과 `rc_tag`를 지정합니다. 정식 버전은 RC 태그에서, 대상 SHA는 `main`의 승격 merge commit에서 결정합니다. 별도 대상 SHA나 버전을 입력하지 않습니다.

1. RC의 출처와 성공 기록, RC manifest artifact 저장 후 24시간 관찰, 정식 출시 간격 168시간을 확인합니다.
2. RC와 소스 트리가 같은 `main` 승격 merge commit의 전체 CI와 공개 설치 검증을 다시 실행합니다.
3. `release-stable` 환경에서 지정 리뷰어의 승인을 기다립니다.
4. 승인 대기 중 채널 HEAD, 정책, 출처, 태그가 바뀌었는지 다시 확인한 뒤 정식 태그를 생성합니다.
5. 정식 태그의 공개 설치까지 성공하면 정식 GitHub Release를 발행하고 manifest를 첨부합니다. 같은 manifest를 Actions artifact에도 저장합니다.

Go 태그는 GitHub Release가 없어도 모듈로 조회될 수 있습니다. 따라서 태그 생성 전에 검증과 승인을 끝냅니다. Preview와 RC는 버전에 `-preview.N`, `-rc.N`을 붙여 구분합니다.

승인 환경은 GitHub 설정에 저장되는 별도 설정입니다. YAML에 이름만 적으면 승인 없는 환경이 자동 생성될 수 있으므로 CI와 발행 코드가 실제 설정을 확인합니다.

- 환경: `release-stable`
- 필수 리뷰어: `NARUBROWN`
- 관리자 승인 우회: 금지
- 실행 브랜치: `main`만 허용
- 자기 실행 승인: 허용. 현재 단일 관리자가 직접 실행하고 승인할 수 있도록 합니다.

환경 승인을 이 대화나 자동화 에이전트가 대신 누르지는 않습니다.

## 공개 검증의 범위

[verify_public.py](scripts/release/verify_public.py)는 임시 디렉터리와 새 Go 모듈 캐시에서 다음을 검사합니다.

- 후보 SHA 또는 태그로 설치한 정확한 Spine 모듈과 고정한 플랫폼 버전
- 코어, 플랫폼의 로컬 replace 부재. MSA 저장소 내부의 공유 이벤트 모듈만 로컬 연결 허용
- HTTP, 폼, WebSocket, DTO 검증과 실제 Kafka, RabbitMQ의 발행, 수신, ACK/NACK E2E
- 고정한 공개 CLI로 HTTP, WebSocket, Kafka 워커, RabbitMQ 워커, 전체 조합 생성
- dry-run과 `--if-match`, 계약 검사, verify, 부분 probe, HTTP 생성 앱 실제 기동과 표시 버전
- 고정한 chat, MSA 공개 커밋의 테스트와 실제 WebSocket, Kafka 왕복

브로커 환경변수가 없으면 공개 검증은 실패합니다. 테스트를 건너뛰고 성공한 것으로 처리하지 않습니다. CLI 부분 probe만으로 브로커나 리스너의 실제 동작을 통과시키지도 않습니다.

GitHub 기본 토큰으로 생성한 태그는 후속 push workflow를 자동으로 실행하지 않습니다. 이 구현은 기존 CI를 `workflow_call`로 직접 호출하고 태그 생성 전후의 공개 검증도 직접 연결합니다.

## 실패와 긴급 수정

- 태그 생성 전 실패: 태그나 릴리스를 만들지 않습니다. 수정한 커밋으로 새 실행을 시작합니다.
- 태그 생성 후 공개 설치 실패: 해당 후보의 승격을 중단합니다. Production에서는 GitHub Release 발행도 중단합니다. 태그를 삭제하거나 덮어쓰지 말고 원인을 확인해 다음 번호 또는 새 패치 버전으로 수정합니다. Go 태그가 이미 공개됐을 가능성을 함께 확인합니다.
- 검증 artifact 저장 실패: workflow가 실패하며 해당 후보는 승격할 수 없습니다. Production에서는 artifact 저장보다 Release 생성이 먼저이므로, 실패한 실행이라도 Release가 이미 생성됐는지 확인합니다.
- 관찰 기간이나 정식 출시 간격만 단축하려면 `observation_override_reason`에 20자 이상의 구체적인 사유를 입력합니다. 사유는 실행 요약과 manifest에 남으며, 정식 릴리스에는 노트로도 남습니다.
- 긴급 수정도 CI, 공개 설치, 출처 확인과 Stable 승인을 생략하지 않습니다.
- 세 공개 workflow는 같은 concurrency 그룹을 사용해 릴리스 발행이 동시에 진행되지 않게 합니다.

정식 태그는 운영상 이 workflow를 통해서만 생성합니다. 저장소의 태그 규칙은 `v*` 태그의 수정, 삭제를 차단합니다. 관리자가 workflow 밖에서 새 태그를 생성하는 것까지 제한하려면 별도의 릴리스 GitHub App 등 전용 발행 주체와 생성 권한 규칙이 필요합니다.

## 로컬 검증

```sh
python3 -m unittest discover -s scripts/release -p 'test_*.py' -v
node --test scripts/release/branch_flow.test.cjs
python3 scripts/release/release.py validate-policy
actionlint -shellcheck= -pyflakes=
```

승인 환경은 `GH_TOKEN`과 `GITHUB_REPOSITORY=NARUBROWN/spine`이 설정된 환경에서 `python3 scripts/release/release.py check-environment`로 읽기 검증할 수 있습니다. 위 검증 명령은 태그나 릴리스를 생성하지 않습니다.
