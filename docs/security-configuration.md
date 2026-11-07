# 보안 설정 참고 자료

Spine은 제한된 런타임 기본값을 사용하며 Kafka·RabbitMQ의 전송 정책은 각 플랫폼 Config가 소유합니다. 배포 검사에서는 `App.Validate`를 호출하십시오. `App.Run`도 네트워크 연결 없이 동일한 사전 검증을 자동으로 수행합니다.

## 기본값

| 영역 | 기본값 | 명시적 개발 또는 호환 설정 |
|---|---|---|
| Kafka | 활성화된 각 리더와 라이터에 TLS 1.2 이상 자동 적용 | 사용자 정의 `TLS`/`Dialer`/`Transport` 또는 로컬 전용 `AllowInsecureTransport: true` |
| RabbitMQ | `amqps://` 필수 | `amqp://`와 함께 `AllowInsecureTransport: true` 사용 |
| RabbitMQ 발행 | persistent, mandatory routing, publisher confirm | `OperationTimeout` 조정; 실패한 메시지는 자동 재발행하지 않음 |
| RabbitMQ 핸들러 실패 | 재큐잉 없이 거부 | `FailurePolicy: rabbitmqplatform.FailureRequeue` |
| RabbitMQ prefetch | consumer당 미확인 메시지 1개 | 양의 `PrefetchCount` |
| 컨슈머 전송 실패 | 지수 백오프와 지터를 적용해 리더 재생성 | `spine.WithEventConsumerRetry` 설정 |
| WebSocket 출처 | 프로토콜과 호스트가 일치해야 함 | 정확한 `AllowedOrigins` 지정 |
| WebSocket 연결 용량 | 활성 및 대기 연결 1024개 | 양의 제한값 또는 `UnlimitedWebSocketConnections` |
| WebSocket 인증 | `WebSocketHandshakeInterceptor` 구현 시 slot/upgrade 전에 실행 | 인증이 필요한 모든 WS route에 handshake 인증 구현 |
| 전역 인터셉터 | HTTP와 WebSocket에 적용 | 더 좁은 범위를 지정하는 `InterceptorFor` |
| 자격 증명 포함 CORS | 명시적 출처 필수 | 없음. 와일드카드와 자격 증명의 조합은 유효하지 않음 |
| 쿠키 직렬화 | 응답을 기록하기 전에 유효하지 않은 필드 거부 | 임의 값을 위한 `EncodeCookieValue` |

## 운영 요구 사항

- 안전하지 않은 브로커 전송은 격리된 로컬 환경에서만 활성화하십시오.
- Spine 컨슈머 큐에 데드 레터 교환기를 설정하기 전에 RabbitMQ에 해당 교환기를 준비하십시오.
- Kafka/RabbitMQ consumer를 멱등하게 구현하십시오. 재연결과 confirm 유실은 정상적인 at-least-once 재전달 또는 중복을 만들 수 있습니다.
- 신뢰하는 프록시 CIDR을 활성화할 때는 클라이언트가 보낸 전달 헤더를 프록시가 제거하거나 덮어쓰도록 설정하십시오.
- `WEBSOCKET_CAPACITY_EXCEEDED`, 컨슈머 재연결 소진, 이벤트 발행 실패 로그를 감시하십시오.
- `boot.ConfigError.Issues` 코드는 안정적인 기계 판독용 배포 진단값으로 취급하고 사람이 읽는 메시지를 파싱하지 마십시오.

[v0.5.1 보정 안내서](migration/v0.5.1.md), [v0.5 마이그레이션 안내서](migration/v0.5.md), [컴파일 가능한 예제](../examples/security-config/main.go)를 참고하십시오.

[이벤트 플랫폼 전환 안내](migration/event-platforms.md)에 App 등록과 제거된 boot 옵션을 정리했습니다.

## CORS 정책

브라우저의 교차 출처 요청을 허용하려면 CORS를 HTTP 전역 인터셉터로 등록하고, 인증 인터셉터보다 앞에 배치하십시오. preflight는 허용 출처, 메서드, 요청 헤더를 확인한 후 204로 끝납니다. 허용되지 않은 preflight는 403으로 끝나며 일반 OPTIONS 요청은 핸들러로 전달됩니다. CORS는 서버 측 인증이나 인가를 대신하지 않습니다.

```go
maxAge := 600
policy, err := cors.NewValidated(cors.Config{
    AllowOrigins:     []string{"https://app.example"},
    AllowMethods:     []string{"GET", "POST"},
    AllowHeaders:     []string{"Content-Type", "Authorization"},
    AllowCredentials: true,
    ExposeHeaders:    []string{"X-Request-ID"},
    MaxAge:           &maxAge,
})
if err != nil {
    return err
}
app.InterceptorFor(boot.InterceptorHTTP, policy)
```

`MaxAge`의 단위는 초입니다. `nil`이면 헤더를 생략하고 0이면 `0`을 출력합니다. 음수는 설정 오류입니다. `AllowOrigin` 콜백은 정적 목록이 허용하지 않은 출처에 대해 호출되며 `(bool, error)`를 반환합니다. 콜백 오류는 요청 오류로 전달됩니다. 정적 목록과 콜백이 모두 없으면 출처를 허용하지 않습니다. `null` 출처는 `*`로 허용되지 않으며 명시적 `null` 또는 콜백으로 허용해야 합니다. 콜백은 신뢰할 수 있는 scheme, host, port를 정확히 검사해야 합니다.

예를 들어 동적 판정에서는 요청의 scheme과 host 및 port를 포함한 전체 출처를 신뢰 목록과 정확히 비교할 수 있습니다.

```go
AllowOrigin: func(ctx context.Context, origin string) (bool, error) {
    return origin == "https://admin.example.com", nil
},
```

credentials가 켜진 설정에서는 출처에 `*`를 사용할 수 없습니다. 메서드, 요청 헤더, 노출 헤더의 `*`는 브라우저 규칙에 따라 credentials 사용 시 와일드카드로 취급되지 않습니다. `Authorization` 요청 헤더는 credentials 여부와 관계없이 `AllowHeaders`에 명시해야 합니다. 성공한 응답에만 허용 출처와 credentials 헤더를 출력합니다. Origin이 없는 요청도 `Vary: Origin`을 추가하며 OPTIONS 응답은 요청 메서드와 요청 헤더에도 `Vary`를 추가합니다. 기존 `Vary`는 보존합니다.

`Config`에 새 필드가 추가됐으므로 위치 기반 구조체 리터럴은 필드 이름을 사용하는 방식으로 변경해야 합니다. 기존 버전에서는 거부된 preflight도 204를 출력하고 일반 응답에도 허용 메서드와 헤더를 출력했습니다. 이제 거부된 preflight는 403이며 허용 메서드와 헤더는 성공한 preflight에만 출력합니다. 라우트에만 등록한 CORS는 대상 GET 등의 정책을 OPTIONS에 자동 적용하지 않습니다. 전역 CORS를 사용하거나 OPTIONS 경로와 정책을 직접 등록하십시오. 전역 정책과 라우트 정책의 자동 우선순위는 제공하지 않습니다. Echo의 native CORS를 쓰는 경우 공통 CORS를 같은 요청에 중복 등록하지 마십시오.
