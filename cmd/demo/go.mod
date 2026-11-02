module github.com/NARUBROWN/spine/cmd/demo

go 1.25.5

require (
	github.com/NARUBROWN/spine v0.5.3-0.20261101161224-490135eda06f
	github.com/NARUBROWN/spine-echo-platform v0.0.0-20261101230311-ded538a25193
	github.com/gorilla/websocket v1.5.3
	github.com/labstack/echo/v5 v5.4.4
)

require (
	github.com/klauspost/compress v1.15.9 // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
	github.com/rabbitmq/amqp091-go v1.10.0 // indirect
	github.com/segmentio/kafka-go v0.4.50 // indirect
)

replace github.com/NARUBROWN/spine => ../..

replace github.com/NARUBROWN/spine-echo-platform => ../../../spine-echo-platform
