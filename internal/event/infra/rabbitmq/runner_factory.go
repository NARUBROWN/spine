package rabbitmq

import (
	"github.com/NARUBROWN/spine/internal/event/consumer"
	"github.com/NARUBROWN/spine/pkg/boot"
)

type RunnerFactory struct {
	opts boot.RabbitMqOptions
}

func NewRunnerFactory(opts boot.RabbitMqOptions) *RunnerFactory {
	return &RunnerFactory{opts: opts}
}

func (f *RunnerFactory) Build(registration consumer.Registration) (consumer.Reader, error) {
	return NewRabbitMqReader(f.opts)
}
