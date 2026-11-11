module github.com/NARUBROWN/spine/test

go 1.25.5

require (
	github.com/NARUBROWN/spine v0.7.5-preview.1
	github.com/NARUBROWN/spine-echo-platform v0.2.1-preview.1
	github.com/NARUBROWN/spine-kafka-platform v0.2.0-preview.2
	github.com/NARUBROWN/spine-rabbitmq-platform v0.2.0-preview.2
	github.com/gorilla/websocket v1.5.3
	github.com/labstack/echo/v5 v5.4.4
	github.com/rabbitmq/amqp091-go v1.10.0
	github.com/segmentio/kafka-go v0.4.50
)

require (
	github.com/NARUBROWN/spine-gorilla-platform v0.2.0-preview.2
	github.com/klauspost/compress v1.15.9 // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
)

replace github.com/NARUBROWN/spine => ../

replace github.com/NARUBROWN/spine-echo-platform => ../../spine-echo-platform
