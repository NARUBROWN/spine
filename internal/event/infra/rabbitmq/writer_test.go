package rabbitmq

import (
	"testing"

	"github.com/NARUBROWN/spine/pkg/boot"
)

func TestNewRabbitMqWriter_RequiresWriteOptions(t *testing.T) {
	_, err := NewRabbitMqWriter(boot.RabbitMqOptions{
		URL: "amqp://guest:guest@localhost:5672/",
	})
	if err == nil {
		t.Fatal("Write 옵션 누락 시 에러가 발생해야 합니다")
	}
}
