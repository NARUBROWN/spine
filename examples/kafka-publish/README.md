# Kafka 주문 이벤트 발행 예제

이 모듈은 코어와 Kafka 어댑터의 개발 소스를 사용합니다. Kafka 어댑터 작업 트리를 코어 작업 트리와 같은 부모 디렉터리의 `spine-kafka-publish-destinations`에 둡니다.

- [destinations.go](destinations/destinations.go)에서 논리 목적지와 발행 목록을 정의합니다.
- [main.go](main.go)의 `BuildApplication`에서 Kafka 설정을 만들고 어댑터를 등록합니다.
- [order_service.go](order_service.go)에서 `publish.Event`에 목적지와 JSON 본문 필드를 넘깁니다.

로컬 Kafka 호환 브로커가 `localhost:19092`에서 실행 중인 상태에서 `dev.orders.created` 토픽을 먼저 생성합니다. Kafka 어댑터가 시작할 때 등록된 토픽의 메타데이터를 확인합니다.

```sh
KAFKA_INSECURE=true KAFKA_TOPIC_PREFIX=dev. go run .
```

```sh
curl -i -X POST http://127.0.0.1:8080/orders/42
```

성공하면 HTTP 202를 반환하고 `dev.orders.created`에 `{"orderId":42}`가 발행됩니다. `KAFKA_BROKERS`로 브로커 주소를 바꿀 수 있습니다. `KAFKA_INSECURE=true`는 로컬 평문 브로커에 연결할 때 사용합니다.
