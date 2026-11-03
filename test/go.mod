module github.com/NARUBROWN/spine/test

go 1.25.5

require (
	github.com/NARUBROWN/spine v0.6.1-0.20261103040400-9b1321410211
	github.com/NARUBROWN/spine-echo-platform v0.1.0
	github.com/NARUBROWN/spine-kafka-platform v0.0.0-20261102223502-7b4626c6f222
	github.com/NARUBROWN/spine-rabbitmq-platform v0.0.0-20261102223515-967e684aff1c
	github.com/gorilla/websocket v1.5.3
	github.com/labstack/echo/v5 v5.4.4
	github.com/rabbitmq/amqp091-go v1.10.0
	github.com/segmentio/kafka-go v0.4.50
)

require (
	github.com/NARUBROWN/spine-gorilla-platform v0.0.0-20261103040512-36ad41dff88f
	github.com/klauspost/compress v1.15.9 // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
)

replace github.com/NARUBROWN/spine => ../

replace github.com/NARUBROWN/spine-echo-platform => ../../spine-echo-platform
