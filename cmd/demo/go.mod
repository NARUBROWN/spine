module github.com/NARUBROWN/spine/cmd/demo

go 1.25.5

require (
	github.com/NARUBROWN/spine v0.5.3-0.20261101153206-39d90e4c871e
	github.com/NARUBROWN/spine-echo-platform v0.0.0-20261101153258-a4a8f6fc184d
	github.com/gorilla/websocket v1.5.3
)

require (
	github.com/klauspost/compress v1.15.9 // indirect
	github.com/labstack/echo/v4 v4.15.0 // indirect
	github.com/labstack/gommon v0.4.2 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
	github.com/rabbitmq/amqp091-go v1.10.0 // indirect
	github.com/segmentio/kafka-go v0.4.50 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasttemplate v1.2.2 // indirect
	golang.org/x/crypto v0.47.0 // indirect
	golang.org/x/net v0.49.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	golang.org/x/text v0.33.0 // indirect
)

replace github.com/NARUBROWN/spine => ../..

replace github.com/NARUBROWN/spine-echo-platform => ../../../spine-echo-platform
