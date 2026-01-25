package boot

/*
Kafka 관련 설정을 담는 옵션 구조체입니다.
Spine 부트스트랩 단계에서 Kafka Producer / Consumer 구성을 제어합니다.
*/
type KafkaOptions struct {
	// Kafka 브로커 주소 목록
	Brokers []string

	/*
		이벤트 소비(Consumer) 설정
		nil이면 Kafka Consumer Runtime은 활성화되지 않습니다.
	*/
	Read *KafkaReadOptions

	/*
		이벤트 발행(Producer) 설정
		nil이면 Kafka로 이벤트를 발행하지 않습니다.
	*/
	Write *KafkaWriteOptions
}

/*
Kafka 이벤트 발행 시 사용되는 설정입니다.
Topic 이름 규칙과 관련된 정책을 정의합니다.
*/
type KafkaWriteOptions struct {
	// 이벤트 이름 앞에 붙일 Topic Prefix
	TopicPrefix string
}

/*
Kafka 이벤트 소비 시 사용되는 설정입니다.
Consumer Group 단위의 실행을 제어합니다.
*/
type KafkaReadOptions struct {
	// Kafka Consumer Group ID
	GroupID string
}
