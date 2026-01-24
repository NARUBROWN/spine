package kafka

import (
	"github.com/NARUBROWN/spine/internal/event/consumer"
	"github.com/NARUBROWN/spine/pkg/boot"
)

type RunnerFactory struct {
	opts boot.KafkaOptions
}

func NewRunnerFactory(opts boot.KafkaOptions) *RunnerFactory {
	return &RunnerFactory{opts: opts}
}

func (f *RunnerFactory) Build(registration consumer.Registration) consumer.Reader {
	return NewKafkaReader(
		registration.Topic,
		f.opts,
	)
}
