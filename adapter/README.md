# 이전 계약 경로 호환성

공개 계약의 정식 경로는 [`contract/http`](../contract/http/README.md), [`contract/event`](../contract/event/README.md), [`contract/validator`](../contract/validator/README.md)입니다.

이 디렉토리는 공개된 플랫폼 버전과 앱이 같은 타입과 오류를 계속 사용할 수 있도록 호환 별칭을 제공합니다.

| 이전 import | 정식 import |
| --- | --- |
| `github.com/NARUBROWN/spine/adapter` | `github.com/NARUBROWN/spine/contract/http` |
| `github.com/NARUBROWN/spine/adapter/http` | `github.com/NARUBROWN/spine/contract/http` |
| `github.com/NARUBROWN/spine/adapter/event` | `github.com/NARUBROWN/spine/contract/event` |

각 경로의 타입·계약 버전·오류는 정식 계약과 같습니다. `ValidateFactory`도 같은 구현을 호출합니다. 기존 Echo·Kafka·RabbitMQ 플랫폼을 사용하기 위해 import를 즉시 바꿀 필요는 없습니다. 새 구현은 정식 경로를 사용하세요.
