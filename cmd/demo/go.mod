module github.com/NARUBROWN/spine/cmd/demo

go 1.25.5

require (
	github.com/NARUBROWN/spine v0.6.1-0.20261103040400-9b1321410211
	github.com/NARUBROWN/spine-echo-platform v0.1.0
	github.com/NARUBROWN/spine-gorilla-platform v0.0.0-20261103040512-36ad41dff88f
	github.com/gorilla/websocket v1.5.3
	github.com/labstack/echo/v5 v5.4.4
)

replace github.com/NARUBROWN/spine => ../..

replace github.com/NARUBROWN/spine-echo-platform => ../../../spine-echo-platform
