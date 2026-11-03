module github.com/NARUBROWN/spine/test/platform-e2e

go 1.26.0

require (
	github.com/NARUBROWN/spine v0.6.1-0.20261103040400-9b1321410211
	github.com/NARUBROWN/spine-echo-platform v0.1.0
	github.com/NARUBROWN/spine-kafka-platform v0.0.0-20261103001523-e4b87708f841
	github.com/NARUBROWN/spine-rabbitmq-platform v0.0.0-20261103001525-b2be17ee8a11
	github.com/NARUBROWN/spine-validator-playground v0.0.0-20261103011253-5508c82a155e
	github.com/gorilla/websocket v1.5.3
	github.com/rabbitmq/amqp091-go v1.10.0
	github.com/segmentio/kafka-go v0.4.50
)

require (
	github.com/NARUBROWN/spine-gorilla-platform v0.0.0-20261103040512-36ad41dff88f
	github.com/gabriel-vasile/mimetype v1.4.16 // indirect
	github.com/go-playground/locales v0.14.2 // indirect
	github.com/go-playground/universal-translator v0.18.2 // indirect
	github.com/go-playground/validator/v10 v10.30.6 // indirect
	github.com/klauspost/compress v1.15.9 // indirect
	github.com/labstack/echo/v5 v5.4.4 // indirect
	github.com/leodido/go-urn v1.5.0 // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

replace github.com/NARUBROWN/spine => ../..
