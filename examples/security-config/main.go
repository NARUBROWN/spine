package main

import (
	"crypto/tls"
	"log"
	"os"
	"time"

	"github.com/NARUBROWN/spine"
	platform "github.com/NARUBROWN/spine-echo-platform"
	kafkaplatform "github.com/NARUBROWN/spine-kafka-platform"
	rabbitmqplatform "github.com/NARUBROWN/spine-rabbitmq-platform"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/interceptor/cors"
	"github.com/NARUBROWN/spine/pkg/boot"
)

type auditInterceptor struct{}

func (*auditInterceptor) PreHandle(core.ExecutionContext, core.HandlerMeta) error { return nil }
func (*auditInterceptor) PostHandle(core.ExecutionContext, core.HandlerMeta)      {}
func (*auditInterceptor) BeforeResponse(core.ExecutionContext, core.HandlerMeta, error) error {
	return nil
}
func (*auditInterceptor) AfterCompletion(core.ExecutionContext, core.HandlerMeta, error) {
}

func secureApp(rabbitURL string) (spine.App, boot.Options, error) {
	sharedTLS := &tls.Config{MinVersion: tls.VersionTLS12}
	kafka, err := kafkaplatform.NewFactory(kafkaplatform.Config{Brokers: []string{"kafka.example:9093"}, TLS: sharedTLS, Consume: true, Publish: true, GroupID: "orders"})
	if err != nil {
		return nil, boot.Options{}, err
	}
	rabbit, err := rabbitmqplatform.NewFactory(rabbitmqplatform.Config{URL: rabbitURL, Exchange: "events", Consume: true, PrefetchCount: 16, FailurePolicy: rabbitmqplatform.FailureReject, DeadLetter: &rabbitmqplatform.DeadLetter{Exchange: "events.dlx", RoutingKey: "events.failed"}})
	if err != nil {
		return nil, boot.Options{}, err
	}
	app := spine.New(platform.Option(),
		spine.WithEventAdapter("kafka", kafka, spine.WithEventConsumerRetry(boot.ConsumerRetryOptions{InitialDelay: 200 * time.Millisecond, MaxDelay: 10 * time.Second, Multiplier: 2, Jitter: 0.2}), spine.WithEventSettlementTimeout(5*time.Second)),
		spine.WithEventAdapter("rabbitmq", rabbit))
	return app, boot.Options{Address: ":8080", HTTP: &boot.HTTPOptions{WebSocket: boot.WebSocketOptions{AllowedOrigins: []string{"https://app.example.com"}, TrustedProxyCIDRs: []string{"10.0.0.0/8"}, MaxConnections: 4096}}}, nil
}

func localInsecureConfigs() (kafkaplatform.Config, rabbitmqplatform.Config) {
	return kafkaplatform.Config{Brokers: []string{"localhost:9092"}, Consume: true, GroupID: "local", AllowInsecureTransport: true},
		rabbitmqplatform.Config{URL: "amqp://guest:guest@localhost:5672/", Exchange: "events", Consume: true, AllowInsecureTransport: true}
}

func main() {
	rabbitURL := os.Getenv("RABBITMQ_URL")
	if rabbitURL == "" {
		log.Fatal("RABBITMQ_URL must contain an amqps:// connection URL")
	}

	app, options, err := secureApp(rabbitURL)
	if err != nil {
		log.Fatal(err)
	}
	app.InterceptorFor(boot.InterceptorAll, &auditInterceptor{})

	corsInterceptor, err := cors.NewValidated(cors.Config{
		AllowOrigins:     []string{"https://app.example.com"},
		AllowMethods:     []string{"GET", "POST"},
		AllowHeaders:     []string{"Authorization", "Content-Type"},
		AllowCredentials: true,
	})
	if err != nil {
		log.Fatal(err)
	}
	app.InterceptorFor(boot.InterceptorHTTP, corsInterceptor)

	if err := app.Validate(options); err != nil {
		log.Fatal(err)
	}

	// 검증 단계에서는 네트워크에 연결하지 않습니다. 실제로 시작하려면 app.Run(options)을 호출하세요.
	_ = localInsecureConfigs
}
