package boot

/*
RabbitMqOptions는 RabbitMQ 사용 여부 및 역할(Read / Write)을 정의합니다.
Read와 Write는 서로 독립적이며, 하나만 설정해도 동작합니다.
*/
type RabbitMqOptions struct {
	/*
		URL은 RabbitMQ AMQP 연결 문자열입니다.
		예: amqp://guest:guest@localhost:5672/
	*/
	URL string

	/*
		Read는 이벤트 소비(Consumer) 설정입니다.
		nil이면 RabbitMQ Consumer Runtime은 활성화되지 않습니다.
	*/
	Read *RabbitMqReadOptions

	/*
		Write는 이벤트 발행(Publisher) 설정입니다.
		nil이면 RabbitMQ로 이벤트를 발행하지 않습니다.
	*/
	Write *RabbitMqWriteOptions
}

/*
RabbitMqReadOptions는 RabbitMQ Consumer(Runtime) 설정입니다.
Queue 선언 및 Exchange 바인딩 책임을 가집니다.
*/
type RabbitMqReadOptions struct {
	// Queue는 메시지를 소비할 큐 이름입니다.
	Queue string

	// Exchange는 큐가 바인딩될 Exchange 이름입니다.
	Exchange string

	// RoutingKey는 Exchange에서 큐로 전달될 메시지의 라우팅 키 조건입니다.
	RoutingKey string
}

/*
RabbitMqWriteOptions는 RabbitMQ Publisher 설정입니다.
Exchange 선언 및 메시지 발행 책임을 가집니다.
*/
type RabbitMqWriteOptions struct {
	// Exchange는 이벤트를 발행할 대상 Exchange 이름입니다.
	Exchange string

	// RoutingKey는 이벤트 발행 시 사용할 라우팅 키입니다.
	RoutingKey string
}
