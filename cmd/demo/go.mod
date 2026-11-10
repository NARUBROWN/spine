module github.com/NARUBROWN/spine/cmd/demo

go 1.25.5

require (
	github.com/NARUBROWN/spine v0.7.2-preview.3
	github.com/NARUBROWN/spine-echo-platform v0.2.1-preview.1
	github.com/NARUBROWN/spine-gorilla-platform v0.2.0-preview.2
	github.com/gorilla/websocket v1.5.3
	github.com/labstack/echo/v5 v5.4.4
)

replace github.com/NARUBROWN/spine => ../..

replace github.com/NARUBROWN/spine-echo-platform => ../../../spine-echo-platform
