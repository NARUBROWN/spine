package main

import (
	"context"
	"fmt"
	"net/http/httptest"

	platform "github.com/NARUBROWN/spine-echo-platform"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/httpx"
)

type controller struct{}

func (*controller) Hello() httpx.Response[string] { return httpx.Response[string]{Body: "hello"} }

func main() {
	app := platform.New()
	app.Constructor(func() *controller { return &controller{} })
	app.Route("GET", "/hello", (*controller).Hello)
	prepared, err := app.Prepare(context.Background(), boot.Options{HTTP: &boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{Enabled: true}}})
	if err != nil {
		panic(err)
	}
	defer prepared.Close(context.Background())
	response := httptest.NewRecorder()
	prepared.Handler().ServeHTTP(response, httptest.NewRequest("GET", "/openapi.json", nil))
	if response.Code != 200 {
		panic(response.Body.String())
	}
	fmt.Print(response.Body.String())
}
