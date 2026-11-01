package bootstrap

import "slices"

// snapshotConfig는 등록 목록과 옵션 값을 복사합니다.
// 생성자, 인터셉터 인스턴스와 사용자 제공 transport/dialer는 복제하지 않습니다.
func snapshotConfig(config Config) Config {
	config.Constructors = slices.Clone(config.Constructors)
	config.Routes = slices.Clone(config.Routes)
	for i := range config.Routes {
		config.Routes[i].Interceptors = slices.Clone(config.Routes[i].Interceptors)
	}
	config.Interceptors = slices.Clone(config.Interceptors)
	config.ScopedInterceptors = slices.Clone(config.ScopedInterceptors)
	config.TransportHooks = slices.Clone(config.TransportHooks)
	config.CustomTransports = slices.Clone(config.CustomTransports)
	if config.ConsumerRegistry != nil {
		config.ConsumerRegistry = config.ConsumerRegistry.Snapshot()
	}
	if config.WebSocketRegistry != nil {
		config.WebSocketRegistry = config.WebSocketRegistry.Snapshot()
	}
	config.HTTP = snapshotValue(config.HTTP)
	if config.HTTP != nil {
		config.HTTP.WebSocket.AllowedOrigins = slices.Clone(config.HTTP.WebSocket.AllowedOrigins)
		config.HTTP.WebSocket.TrustedProxyCIDRs = slices.Clone(config.HTTP.WebSocket.TrustedProxyCIDRs)
	}
	config.Kafka = snapshotValue(config.Kafka)
	if config.Kafka != nil {
		config.Kafka.Brokers = slices.Clone(config.Kafka.Brokers)
		config.Kafka.Read = snapshotValue(config.Kafka.Read)
		config.Kafka.Write = snapshotValue(config.Kafka.Write)
		if config.Kafka.TLS != nil {
			config.Kafka.TLS = config.Kafka.TLS.Clone()
		}
	}
	config.RabbitMQ = snapshotValue(config.RabbitMQ)
	if config.RabbitMQ != nil {
		config.RabbitMQ.Read = snapshotValue(config.RabbitMQ.Read)
		config.RabbitMQ.Write = snapshotValue(config.RabbitMQ.Write)
		if config.RabbitMQ.Read != nil {
			config.RabbitMQ.Read.DeadLetter = snapshotValue(config.RabbitMQ.Read.DeadLetter)
		}
	}
	return config
}

func snapshotValue[T any](value *T) *T {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
