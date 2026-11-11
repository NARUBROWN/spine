package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/NARUBROWN/spine"
	platform "github.com/NARUBROWN/spine-echo-platform"
	gorillaplatform "github.com/NARUBROWN/spine-gorilla-platform"
	kafkaplatform "github.com/NARUBROWN/spine-kafka-platform"
	rabbitmqplatform "github.com/NARUBROWN/spine-rabbitmq-platform"
	"github.com/NARUBROWN/spine/interceptor/cors"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/constructor"
)

func main() {
	config, err := buildDemoConfig()
	if err != nil {
		log.Fatalf("[Demo] invalid environment configuration: %v", err)
	}
	appOptions := []spine.Option{platform.Option(), gorillaplatform.Option()}
	if config.Kafka != nil {
		factory, err := kafkaplatform.NewFactory(*config.Kafka)
		if err != nil {
			log.Fatal(err)
		}
		appOptions = append(appOptions, spine.WithEventAdapter("kafka", factory))
	}
	if config.RabbitMQ != nil {
		factory, err := rabbitmqplatform.NewFactory(*config.RabbitMQ)
		if err != nil {
			log.Fatal(err)
		}
		appOptions = append(appOptions, spine.WithEventAdapter("rabbitmq", factory))
	}
	app := spine.New(appOptions...)
	publishEnabled := (config.Kafka != nil && config.Kafka.Publish) || (config.RabbitMQ != nil && config.RabbitMQ.Publish)

	app.Constructor(
		constructor.Controller(&UserController{publishEnabled: publishEnabled}),
		constructor.Controller(&CommonController{}),
		constructor.Custom(NewOrderConsumer),
		constructor.Custom(NewChatController),
	)

	corsMaxAge := 600
	corsInterceptor, err := cors.NewValidated(cors.Config{
		AllowOrigins:     []string{config.AllowedOrigin},
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:     []string{"Authorization", "Content-Type"},
		AllowCredentials: true,
		ExposeHeaders:    []string{"X-Request-ID"},
		MaxAge:           &corsMaxAge,
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
	Kafka         *kafkaplatform.Config
	RabbitMQ      *rabbitmqplatform.Config
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

	kafka := demoKafkaOptions(allowInsecure)
	rabbit := demoRabbitMqOptions(allowInsecure)
	return demoConfig{Options: options, AllowedOrigin: allowedOrigin, Kafka: kafka, RabbitMQ: rabbit}, nil
}

func demoKafkaOptions(allowInsecure bool) *kafkaplatform.Config {
	brokers := splitListEnv("SPINE_DEMO_KAFKA_BROKERS")
	if len(brokers) == 0 {
		return nil
	}
	return &kafkaplatform.Config{Brokers: brokers, Consume: true, Publish: true,
		GroupID:     envOrDefault("SPINE_DEMO_KAFKA_GROUP_ID", "spine-demo-consumer"),
		TopicPrefix: strings.TrimSpace(os.Getenv("SPINE_DEMO_KAFKA_TOPIC_PREFIX")), AllowInsecureTransport: allowInsecure}
}

func demoRabbitMqOptions(allowInsecure bool) *rabbitmqplatform.Config {
	address := strings.TrimSpace(os.Getenv("SPINE_DEMO_RABBITMQ_URL"))
	if address == "" {
		return nil
	}
	config := &rabbitmqplatform.Config{URL: address, Exchange: envOrDefault("SPINE_DEMO_RABBITMQ_EXCHANGE", "stock-exchange"), Consume: true, Publish: true,
		AllowInsecureTransport: allowInsecure, FailurePolicy: rabbitmqplatform.FailureReject}
	if dlx := strings.TrimSpace(os.Getenv("SPINE_DEMO_RABBITMQ_DLX")); dlx != "" {
		config.DeadLetter = &rabbitmqplatform.DeadLetter{Exchange: dlx, RoutingKey: strings.TrimSpace(os.Getenv("SPINE_DEMO_RABBITMQ_DLX_ROUTING_KEY"))}
	}
	return config
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
