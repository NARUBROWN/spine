package consumer

type Message struct {
	EventName string
	Payload   []byte
	Metadata  map[string]string

	// ACK/NACK 콜백 함수 (선택적)
	// Reader 구현체에서 설정하며, Runtime에서 처리 결과에 따라 호출
	ack  func() error
	nack func() error
}

// Ack는 메시지 처리 성공을 메시지 브로커에 알립니다.
func (m *Message) Ack() error {
	if m.ack != nil {
		return m.ack()
	}
	return nil
}

// Nack는 메시지 처리 실패를 메시지 브로커에 알립니다.
func (m *Message) Nack() error {
	if m.nack != nil {
		return m.nack()
	}
	return nil
}

// SetAckHandler는 ACK 콜백 함수를 설정합니다 (Reader 구현체용).
func (m *Message) SetAckHandler(ack func() error) {
	m.ack = ack
}

// SetNackHandler는 NACK 콜백 함수를 설정합니다 (Reader 구현체용).
func (m *Message) SetNackHandler(nack func() error) {
	m.nack = nack
}
