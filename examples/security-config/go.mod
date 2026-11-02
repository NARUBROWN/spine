module github.com/NARUBROWN/spine/examples/security-config

go 1.25.5

require (
	github.com/NARUBROWN/spine v0.5.3-0.20261102012946-eed61dc88259
	github.com/NARUBROWN/spine-echo-platform v0.0.0-20261102013128-0a80ce357ba5
)

require (
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/klauspost/compress v1.15.9 // indirect
	github.com/labstack/echo/v5 v5.4.4 // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
	github.com/rabbitmq/amqp091-go v1.10.0 // indirect
	github.com/segmentio/kafka-go v0.4.50 // indirect
)

replace github.com/NARUBROWN/spine => ../../

replace github.com/NARUBROWN/spine-echo-platform => ../../../spine-echo-platform
