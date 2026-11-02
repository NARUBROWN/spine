package main

import (
	"testing"

	"github.com/NARUBROWN/spine/pkg/boot"
)

func TestBuildDemoConfig_DefaultsToHTTPAndWebSocketOnly(t *testing.T) {
	clearDemoEnv(t)
	config, err := buildDemoConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.Options.Kafka != nil || config.Options.RabbitMQ != nil {
		t.Fatal("brokers must remain disabled until explicitly configured")
	}
	if config.AllowedOrigin != "http://localhost:5173" {
		t.Fatalf("default allowed origin = %q", config.AllowedOrigin)
	}
	ws := config.Options.HTTP.WebSocket
	if ws.MaxConnections != 0 || len(ws.AllowedOrigins) != 1 {
		t.Fatalf("unexpected WebSocket defaults: %+v", ws)
	}
	if err := config.Options.HTTP.Validate(); err != nil {
		t.Fatalf("default HTTP/WS configuration must pass preflight: %v", err)
	}
}

func TestBuildDemoConfig_ReliesOnSecureKafkaDefault(t *testing.T) {
	clearDemoEnv(t)
	t.Setenv("SPINE_DEMO_KAFKA_BROKERS", "kafka-a:9093, kafka-b:9093")
	config, err := buildDemoConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.Options.Kafka == nil || config.Options.Kafka.TLS != nil {
		t.Fatal("the reference app must leave default TLS policy to Spine")
	}
	if config.Options.Kafka.AllowInsecureTransport {
		t.Fatal("insecure Kafka must require an explicit opt-in")
	}
	if err := config.Options.Kafka.Validate(); err != nil {
		t.Fatalf("secure Kafka demo configuration must pass preflight: %v", err)
	}
}

func TestBuildDemoConfig_RequiresExplicitLocalBrokerOptIn(t *testing.T) {
	clearDemoEnv(t)
	t.Setenv("SPINE_DEMO_ALLOW_INSECURE_BROKERS", "true")
	t.Setenv("SPINE_DEMO_KAFKA_BROKERS", "localhost:9092")
	t.Setenv("SPINE_DEMO_RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")
	t.Setenv("SPINE_DEMO_RABBITMQ_DLX", "events.dlx")
	config, err := buildDemoConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.Options.Kafka.TLS != nil || !config.Options.Kafka.AllowInsecureTransport {
		t.Fatalf("unexpected local Kafka configuration: %+v", config.Options.Kafka)
	}
	if config.Options.RabbitMQ == nil || !config.Options.RabbitMQ.AllowInsecureTransport {
		t.Fatalf("unexpected local RabbitMQ configuration: %+v", config.Options.RabbitMQ)
	}
	read := config.Options.RabbitMQ.Read
	if read.EffectiveFailurePolicy() != boot.RabbitMqFailureReject || read.DeadLetter == nil || read.DeadLetter.Exchange != "events.dlx" {
		t.Fatalf("RabbitMQ safe default and optional DLX must be preserved: %+v", read)
	}
	if err := config.Options.Kafka.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := config.Options.RabbitMQ.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestBuildDemoConfig_RejectsInvalidBooleanEnvironment(t *testing.T) {
	clearDemoEnv(t)
	t.Setenv("SPINE_DEMO_ALLOW_INSECURE_BROKERS", "sometimes")
	if _, err := buildDemoConfig(); err == nil {
		t.Fatal("invalid insecure transport opt-in must fail")
	}
}

func TestBuildDemoConfig_LeavesInvalidTrustedProxyForPreflight(t *testing.T) {
	clearDemoEnv(t)
	t.Setenv("SPINE_DEMO_TRUSTED_PROXY_CIDRS", "not-a-cidr")
	config, err := buildDemoConfig()
	if err != nil {
		t.Fatal(err)
	}
	if err := config.Options.HTTP.Validate(); err == nil {
		t.Fatal("invalid trusted proxy CIDR must fail the 0.5 preflight")
	}
}

func clearDemoEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"SPINE_DEMO_ADDRESS",
		"SPINE_DEMO_ALLOWED_ORIGIN",
		"SPINE_DEMO_TRUSTED_PROXY_CIDRS",
		"SPINE_DEMO_ALLOW_INSECURE_BROKERS",
		"SPINE_DEMO_KAFKA_BROKERS",
		"SPINE_DEMO_KAFKA_GROUP_ID",
		"SPINE_DEMO_KAFKA_TOPIC_PREFIX",
		"SPINE_DEMO_RABBITMQ_URL",
		"SPINE_DEMO_RABBITMQ_EXCHANGE",
		"SPINE_DEMO_RABBITMQ_DLX",
		"SPINE_DEMO_RABBITMQ_DLX_ROUTING_KEY",
	} {
		t.Setenv(name, "")
	}
}
