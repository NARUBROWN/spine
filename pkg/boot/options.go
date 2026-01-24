package boot

import "time"

type Options struct {
	Address                string
	EnableGracefulShutdown bool
	ShutdownTimeout        time.Duration
	Kafka                  *KafkaOptions // nil이면 미사용
}
