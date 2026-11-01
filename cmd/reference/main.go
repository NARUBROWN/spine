package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/NARUBROWN/spine"
	"github.com/NARUBROWN/spine/interceptor/cors"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/route"
)

func main() {
	app := spine.New()
	config, err := buildDemoConfig()
	if err != nil {
		log.Fatalf("[Demo] invalid environment configuration: %v", err)
	}

	// 생성자 등록
	app.Constructor(
		NewUserController,
		NewOrderConsumer,
		NewCommonController,
		NewChatController,
	)

	// 라우트 등록, 라우터 단위 인터셉터
	app.Route(
		"GET",
		"/users/:id",
		(*UserController).GetUser,
		route.WithInterceptors(&LoggingInterceptor{}),
	)

	app.Route(
		"GET",
		"/users",
		(*UserController).GetUserQuery,
	)

	app.Route(
		"POST",
		"/upload",
		(*UserController).Upload,
	)

	app.Route(
		"POST",
		"/orders/:orderId",
		(*UserController).CreateOrder,
	)

	app.Route(
		"POST",
		"/stocks/:stockId",
		(*UserController).CreateStock,
	)

	app.Route(
		"GET",
		"/headers",
		(*CommonController).CheckHeader,
	)

	app.Route(
		"GET",
		"/images/:id",
		(*CommonController).GetAvatar,
	)

	corsInterceptor, err := cors.NewValidated(cors.Config{
		AllowOrigins:     []string{config.AllowedOrigin},
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:     []string{"Authorization", "Content-Type"},
		AllowCredentials: true,
	})
	if err != nil {
		log.Fatalf("[Demo] invalid CORS configuration: %v", err)
	}
	app.InterceptorFor(boot.InterceptorHTTP, corsInterceptor)

	if err := app.Consumers().Register(
		"order.created",
		(*OrderConsumer).OnCreatedKafka,
	); err != nil {
		panic(err)
	}

	if err := app.Consumers().Register(
		"stock.created",
		(*OrderConsumer).OnCreatedRabbitMQ,
	); err != nil {
		panic(err)
	}

	if err := app.WebSocket().Register("/ws/chat", (*ChatController).OnMessage); err != nil {
		panic(err)
	}

	if err := app.Validate(config.Options); err != nil {
		log.Fatalf("[Demo] preflight validation failed: %v", err)
	}
	if err := app.Run(config.Options); err != nil {
		log.Fatalf("[Demo] application stopped with an error: %v", err)
	}
}

type demoConfig struct {
	Options       boot.Options
	AllowedOrigin string
}

func buildDemoConfig() (demoConfig, error) {
	allowInsecure, err := optionalBoolEnv("SPINE_DEMO_ALLOW_INSECURE_BROKERS")
	if err != nil {
		return demoConfig{}, err
	}

	allowedOrigin := envOrDefault("SPINE_DEMO_ALLOWED_ORIGIN", "http://localhost:5173")
	options := boot.Options{
		Address:                envOrDefault("SPINE_DEMO_ADDRESS", ":8080"),
		EnableGracefulShutdown: true,
		HTTP: &boot.HTTPOptions{
			WebSocket: boot.WebSocketOptions{
				AllowedOrigins:    []string{allowedOrigin},
				TrustedProxyCIDRs: splitListEnv("SPINE_DEMO_TRUSTED_PROXY_CIDRS"),

				// 선택 사항: 아래 설정을 생략하면 Spine이 제한된 범위의 기본값을 사용합니다.
				// 최대 연결 수 설정 예시: MaxConnections: 4096,
				// 최대 메시지 크기 설정 예시: MaxMessageBytes: 2 << 20,
				// 추가 조정 항목: CapacityRetryAfter, HandshakeTimeout, ReadTimeout,
				// WriteTimeout, PingInterval도 필요에 맞게 조정할 수 있습니다.
			},

			// 선택 사항: 아래 설정을 생략하면 Spine이 제한된 범위의 HTTP 기본값을 사용합니다.
			// 읽기 제한 시간 설정 예시: ReadTimeout: 45 * time.Second,
			// 최대 본문 크기 설정 예시: MaxBodyBytes: 64 << 20,
		},

		// 선택 사항: ShutdownTimeout을 지정하지 않으면 Spine의 기본 정상 종료 제한 시간을 사용합니다.
		// 정상 종료 제한 시간 설정 예시: ShutdownTimeout: 20 * time.Second,
	}

	options.Kafka = demoKafkaOptions(allowInsecure)
	options.RabbitMQ = demoRabbitMqOptions(allowInsecure)

	return demoConfig{Options: options, AllowedOrigin: allowedOrigin}, nil
}

func demoKafkaOptions(allowInsecure bool) *boot.KafkaOptions {
	brokers := splitListEnv("SPINE_DEMO_KAFKA_BROKERS")
	if len(brokers) == 0 {
		return nil
	}
	return &boot.KafkaOptions{
		Brokers:                brokers,
		AllowInsecureTransport: allowInsecure,
		Read: &boot.KafkaReadOptions{
			GroupID: envOrDefault("SPINE_DEMO_KAFKA_GROUP_ID", "spine-demo-consumer"),
		},
		Write: &boot.KafkaWriteOptions{
			TopicPrefix: strings.TrimSpace(os.Getenv("SPINE_DEMO_KAFKA_TOPIC_PREFIX")),
		},

		// 선택 사항: TLS가 nil이면 Spine의 TLS 1.2 이상 기본 설정을 사용합니다.
		// 사용자 정의 CA, mTLS, SASL 또는 연결 방식을 쓸 때만 TLS, Dialer, Transport를 설정하세요.
		// 선택 사항: ConsumerRetry를 지정하지 않으면 100ms~5s 지수 백오프,
		// 배수 2, 지터 20%, 무제한 재시도를 사용합니다.
		// 최대 재시도 횟수 설정 예시: ConsumerRetry: boot.ConsumerRetryOptions{MaxAttempts: 20},
	}
}

func demoRabbitMqOptions(allowInsecure bool) *boot.RabbitMqOptions {
	rabbitURL := strings.TrimSpace(os.Getenv("SPINE_DEMO_RABBITMQ_URL"))
	if rabbitURL == "" {
		return nil
	}
	exchange := envOrDefault("SPINE_DEMO_RABBITMQ_EXCHANGE", "stock-exchange")
	read := &boot.RabbitMqReadOptions{
		Exchange: exchange,

		// 선택 사항: FailurePolicy를 지정하지 않으면 재큐잉하지 않고 안전하게 거부합니다.
		// 실패 메시지 재큐잉 설정 예시: FailurePolicy: boot.RabbitMqFailureRequeue,
	}
	if dlx := strings.TrimSpace(os.Getenv("SPINE_DEMO_RABBITMQ_DLX")); dlx != "" {
		read.DeadLetter = &boot.RabbitMqDeadLetterOptions{
			Exchange:   dlx,
			RoutingKey: strings.TrimSpace(os.Getenv("SPINE_DEMO_RABBITMQ_DLX_ROUTING_KEY")),
		}
	}
	return &boot.RabbitMqOptions{
		URL:                    rabbitURL,
		AllowInsecureTransport: allowInsecure,
		Read:                   read,
		Write:                  &boot.RabbitMqWriteOptions{Exchange: exchange},

		// 선택 사항: ConsumerRetry를 지정하지 않으면 Kafka와 동일한 제한 범위의 재시도 기본값을 사용합니다.
		// 최대 재시도 횟수 설정 예시: ConsumerRetry: boot.ConsumerRetryOptions{MaxAttempts: 20},
		// 선택 사항: FailurePolicy를 지정하지 않으면 RabbitMqFailureReject를 사용합니다.
		// 반복 전달이 의도된 경우에만 RabbitMqFailureRequeue를 설정하세요.
	}
}

func optionalBoolEnv(name string) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return false, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean: %w", name, err)
	}
	return value, nil
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func splitListEnv(name string) []string {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			values = append(values, value)
		}
	}
	return values
}
