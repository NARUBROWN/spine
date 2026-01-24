package consumer

type Message struct {
	EventName string
	Payload   []byte
	Metadata  map[string]string
}
