# Spine v0.5 참고 애플리케이션

`cmd/reference`는 기존 종합 demo의 기능과 v0.5의 안전한 기본 설정 계약을 보여 줍니다.
CLI 계약과 최소 E2E 예제는 [cmd/demo](../demo/README.md)를 사용합니다.

- `Run` 전에 네트워크 연결 없이 수행하는 `App.Validate` 사전 검증
- 명시적 출처를 사용하는 자격 증명 포함 CORS 검증
- HTTP에만 적용되는 CORS 인터셉터 범위
- 제한된 HTTP 및 WebSocket 기본값과 코드에 문서화된 선택적 조정 방법
- Kafka TLS 1.2 이상 자동 적용과 안전하지 않은 로컬 연결의 명시적 허용
- RabbitMQ 메시지 거부 기본값과 선택적 데드 레터 교환기
- 제한된 컨슈머 재연결 기본값과 코드에 문서화된 선택적 조정 방법

이 애플리케이션은 브로커 없이 HTTP/WebSocket 서비스로 실행됩니다.

```sh
go run ./cmd/reference
```

기본 브라우저 출처는 `http://localhost:5173`입니다. 필요한 경우 다음과 같이 변경합니다.

```sh
SPINE_DEMO_ALLOWED_ORIGIN=https://app.example.com go run ./cmd/reference
```

운영 환경용 Kafka 연결에는 TLS가 자동으로 적용됩니다.

```sh
SPINE_DEMO_KAFKA_BROKERS=kafka-a.example:9093,kafka-b.example:9093 \
go run ./cmd/reference
```

운영 환경의 RabbitMQ에는 `amqps://` URL을 사용해야 합니다. 자격 증명은 소스 관리에 포함하지 마십시오.

```sh
SPINE_DEMO_RABBITMQ_URL="$RABBITMQ_URL" \
SPINE_DEMO_RABBITMQ_DLX=events.dlx \
SPINE_DEMO_RABBITMQ_DLX_ROUTING_KEY=events.failed \
go run ./cmd/reference
```

평문 브로커 연결은 로컬 개발 환경에서 명시적으로 허용해야 합니다.

```sh
SPINE_DEMO_ALLOW_INSECURE_BROKERS=true \
SPINE_DEMO_KAFKA_BROKERS=localhost:9092 \
SPINE_DEMO_RABBITMQ_URL=amqp://guest:guest@localhost:5672/ \
go run ./cmd/reference
```

신뢰하는 리버스 프록시에서 TLS를 종료한다면 정확한 프록시 CIDR을 설정합니다.

```sh
SPINE_DEMO_TRUSTED_PROXY_CIDRS=10.0.0.0/8,192.168.0.0/16 \
go run ./cmd/reference
```

프록시는 클라이언트가 보낸 `Forwarded` 및 `X-Forwarded-Proto` 헤더를 제거하거나 덮어써야 합니다.
