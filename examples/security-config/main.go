package main

import (
	"crypto/tls"
	"log"
	"os"
	"time"

	platform "github.com/NARUBROWN/spine-echo-platform"
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

func secureOptions(rabbitURL string) boot.Options {
	sharedTLS := &tls.Config{MinVersion: tls.VersionTLS12}
	return boot.Options{
		Address: ":8080",
		Kafka: &boot.KafkaOptions{
			Brokers: []string{"kafka.example:9093"},
			TLS:     sharedTLS,
			Read:    &boot.KafkaReadOptions{GroupID: "orders"},
			Write:   &boot.KafkaWriteOptions{},
			ConsumerRetry: boot.ConsumerRetryOptions{
				InitialDelay: 200 * time.Millisecond,
				MaxDelay:     10 * time.Second,
				Multiplier:   2,
				Jitter:       0.2,
			},
		},
		RabbitMQ: &boot.RabbitMqOptions{
			URL: rabbitURL,
			Read: &boot.RabbitMqReadOptions{
				Exchange:      "events",
				PrefetchCount: 16,
				FailurePolicy: boot.RabbitMqFailureReject,
				DeadLetter: &boot.RabbitMqDeadLetterOptions{
					Exchange:   "events.dlx",
					RoutingKey: "events.failed",
				},
			},
		},
		HTTP: &boot.HTTPOptions{
			WebSocket: boot.WebSocketOptions{
				AllowedOrigins:    []string{"https://app.example.com"},
				TrustedProxyCIDRs: []string{"10.0.0.0/8"},
				MaxConnections:    4096,
			},
		},
	}
}

func localInsecureOptions() boot.Options {
	return boot.Options{
		Kafka: &boot.KafkaOptions{
			Brokers:                []string{"localhost:9092"},
			AllowInsecureTransport: true,
			Read:                   &boot.KafkaReadOptions{GroupID: "local"},
		},
		RabbitMQ: &boot.RabbitMqOptions{
			URL:                    "amqp://guest:guest@localhost:5672/",
			AllowInsecureTransport: true,
		},
	}
}

func main() {
	rabbitURL := os.Getenv("RABBITMQ_URL")
	if rabbitURL == "" {
		log.Fatal("RABBITMQ_URL must contain an amqps:// connection URL")
	}

	app := platform.New()
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

	if err := app.Validate(secureOptions(rabbitURL)); err != nil {
		log.Fatal(err)
	}

	// 검증 단계에서는 네트워크에 연결하지 않습니다. 실제로 시작하려면 app.Run(secureOptions(rabbitURL))을 호출하세요.
	_ = localInsecureOptions
}
