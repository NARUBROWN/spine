package boot

type KafkaOptions struct {
	Brokers []string

	Read  *KafkaReadOptions
	Write *KafkaWriteOptions
}

type KafkaWriteOptions struct {
	TopicPrefix string
}

type KafkaReadOptions struct {
	GroupID string
}
