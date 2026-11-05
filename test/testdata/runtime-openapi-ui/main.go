package main

import (
	"context"
	"fmt"
	"github.com/NARUBROWN/spine"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/NARUBROWN/spine"
	platform "github.com/NARUBROWN/spine-echo-platform"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/openapi/ui/redoc"
	"github.com/NARUBROWN/spine/pkg/openapi/ui/swaggerui"
)

type controller struct{}

func (*controller) Hello() httpx.Response[string] {
	return httpx.Response[string]{Body: "hello from Spine"}
}

func main() {
	app := spine.New(platform.Option())
	app.Constructor(func() *controller { return &controller{} })
	app.Route("GET", "/hello", (*controller).Hello)
	prepared, err := app.Prepare(context.Background(), boot.Options{HTTP: &boot.HTTPOptions{OpenAPI: boot.OpenAPIOptions{
		Enabled: true,
		Path:    os.Getenv("SPINE_DOCS_PROBE_DOCUMENT_PATH"),
		Title:   "Spine embedded docs probe",
		UI:      []boot.OpenAPIUI{swaggerui.New(swaggerui.Options{}), redoc.New(redoc.Options{})},
	}}})
	if err != nil {
		log.Fatal(err)
	}
	defer prepared.Close(context.Background())
	handler, err := spine.NetHTTPHandler(prepared)
	if err != nil {
		panic(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/service/", http.StripPrefix("/service", handler))
	mux.Handle("/", handler)
	address := os.Getenv("SPINE_DOCS_PROBE_ADDR")
	if address == "" {
		address = "127.0.0.1:0"
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("http://" + listener.Addr().String())
	log.Fatal(http.Serve(listener, mux))
}
