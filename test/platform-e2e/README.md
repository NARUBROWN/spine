# 공개 플랫폼 계약 E2E

실제 Echo·go-playground·Kafka·RabbitMQ 구현을 하나의 Spine 계약 조합으로 검증합니다. 공개된 구현체 버전을 고정하고, Spine만 현재 소스로 연결합니다. 엔진과 플랫폼을 코어 의존성에 추가하지 않기 위해 별도 Go 모듈로 유지합니다.

go-playground v10.30.6을 사용하는 검증 구현은 Go 1.26 이상이 필요합니다. Spine 코어의 최소 Go 버전은 별도로 유지됩니다.

```sh
cd test/platform-e2e
GOWORK=off go test -race -v ./... -count=1 -timeout=3m
```

브로커 환경 변수를 생략하면 Kafka·RabbitMQ 하위 테스트는 건너뜁니다. HTTP JSON·폼, WebSocket, 검증 계약 검사는 계속 실행합니다. 전체 E2E에는 다음 두 변수가 모두 필요합니다.

```sh
export SPINE_TEST_KAFKA_BROKER=127.0.0.1:19092
export SPINE_TEST_RABBITMQ_URL=amqp://guest:guest@127.0.0.1:5672/
GOWORK=off go test -race -v ./... -count=1 -timeout=3m
```

`TestFinalE2EHTTPValidatorBrokerConsumer`는 각 브로커에서 다음을 확인합니다.

1. HTTP DTO 규칙 위반은 핸들러 실행 전에 400으로 차단됩니다.
2. 정상 요청은 이벤트를 발행하고, 컨슈머의 DTO 검증과 핸들러·결과 발행을 마친 뒤 ACK됩니다.
3. HTTP 검증을 우회한 외부 생산자의 잘못된 DTO도 컨슈머 검증에서 거부되고 NACK됩니다.
4. 두 앱과 native 자원이 정상 종료됩니다.

테스트는 고유 topic·queue·exchange를 만들고 직접 만든 자원만 정리합니다. RabbitMQ는 잘못된 입력을 reject하며 Kafka는 실패한 offset 뒤의 커밋을 막는 reader 무효화 정책을 사용합니다. 이 검증은 DB와 브로커의 원자적 처리나 exactly-once를 의미하지 않습니다.

로컬 소스 연결과 공개 모듈 설치는 서로 다른 증거입니다. 공개 버전만 재검증하려면 이 디렉토리를 저장소 밖의 새 폴더에 복사하고 `go mod edit -dropreplace=github.com/NARUBROWN/spine`로 로컬 Spine 연결을 제거한 뒤 같은 명령을 실행합니다.

## 2026-11-03 공개 버전 검증

저장소 밖의 새 프로젝트에서 다음 버전을 설치하고 전체 모듈에 `replace`가 없는 것을 확인한 뒤 race 옵션과 실제 두 브로커로 전체 테스트를 통과했습니다. 브로커 하위 테스트도 생략하지 않았습니다.

| 모듈 | 검증 버전 |
| --- | --- |
| Spine | `v0.6.1-0.20261103010611-6e60b34204d3` |
| Echo 플랫폼 | `v0.1.0` |
| Kafka 플랫폼 | `v0.0.0-20261103001523-e4b87708f841` |
| RabbitMQ 플랫폼 | `v0.0.0-20261103001525-b2be17ee8a11` |
| go-playground 구현 | `v0.0.0-20261103011253-5508c82a155e` |

검증에는 Redpanda `v24.3.18`, RabbitMQ `3-management`, Go `1.26.0`을 사용했습니다. 일반 입력·폼 바인딩·잘못된 JSON·잘못된 규칙의 처리 오류, WebSocket 정상 메시지와 1008 종료, 중첩 맵 경로와 결정적인 오류 순서, 두 브로커의 정상 결과와 거부 경로를 확인했습니다. 이 결과는 위 버전 조합의 로컬 실행 증거이며 PR 병합이나 정식 태그 릴리스를 뜻하지 않습니다.
