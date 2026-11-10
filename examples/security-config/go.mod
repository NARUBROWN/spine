module github.com/NARUBROWN/spine/examples/security-config

go 1.25.5

require (
	github.com/NARUBROWN/spine v0.7.2-preview.3
	github.com/NARUBROWN/spine-echo-platform v0.2.1-preview.1
	github.com/NARUBROWN/spine-kafka-platform v0.2.0-preview.2
	github.com/NARUBROWN/spine-rabbitmq-platform v0.2.0-preview.2
)

require (
	github.com/klauspost/compress v1.15.9 // indirect
	github.com/labstack/echo/v5 v5.4.4 // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
	github.com/rabbitmq/amqp091-go v1.10.0 // indirect
	github.com/segmentio/kafka-go v0.4.50 // indirect
)

replace github.com/NARUBROWN/spine => ../../

replace github.com/NARUBROWN/spine-echo-platform => ../../../spine-echo-platform
