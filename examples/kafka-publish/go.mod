module github.com/NARUBROWN/spine/examples/kafka-publish

go 1.25.5

require (
	github.com/NARUBROWN/spine v0.7.5
	github.com/NARUBROWN/spine-echo-platform v0.2.1
	github.com/NARUBROWN/spine-kafka-platform v0.2.0
)

require (
	github.com/klauspost/compress v1.15.9 // indirect
	github.com/labstack/echo/v5 v5.4.4 // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
	github.com/segmentio/kafka-go v0.4.50 // indirect
)

replace github.com/NARUBROWN/spine => ../..

replace github.com/NARUBROWN/spine-kafka-platform => ../../../spine-kafka-publish-destinations
