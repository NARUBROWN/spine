package main

import (
	"log"
	"net/http"

	"os"
	"strings"

	"github.com/NARUBROWN/spine"
	echo "github.com/NARUBROWN/spine-echo-platform"
	kafka "github.com/NARUBROWN/spine-kafka-platform"
	"github.com/NARUBROWN/spine/examples/kafka-publish/destinations"
	"github.com/NARUBROWN/spine/pkg/api"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/constructor"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/input"
	ctxspine "github.com/NARUBROWN/spine/pkg/spine"
)

func BuildApplication() (spine.App, error) {
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		brokers = "localhost:19092"
	}
	factory, err := kafka.NewFactory(kafka.Config{
		Brokers:                strings.Split(brokers, ","),
		Publish:                true,
		TopicPrefix:            os.Getenv("KAFKA_TOPIC_PREFIX"),
		PublishDestinations:    destinations.Catalog{},
		AllowInsecureTransport: os.Getenv("KAFKA_INSECURE") == "true",
	})
	if err != nil {
		return nil, err
	}
	app := spine.New(echo.Option(), spine.WithEventAdapter("kafka", factory))
	app.Constructor(
		constructor.Service(&OrderService{}),
		constructor.Controller(&OrderController{}),
	)
	return app, nil
}

type OrderController struct{ service *OrderService }

func (*OrderController) Constructor(service *OrderService) *OrderController {
	return &OrderController{service: service}
}

func (c *OrderController) RegisterRoutes(routes *api.Registry) {
	routes.POST("/orders/:id", c.Create)
}

type OrderPath struct {
	ID int64 `path:"id"`
}

func (c *OrderController) Create(ctx ctxspine.Ctx, order input.Path[OrderPath]) (httpx.Response[string], error) {
	if err := c.service.Create(ctx.ExecutionContext().Context(), order.ID); err != nil {
		return httpx.Response[string]{}, err
	}
	return httpx.Response[string]{Body: "accepted", Options: httpx.ResponseOptions{Status: http.StatusAccepted}}, nil
}

func main() {
	app, err := BuildApplication()
	if err != nil {
		log.Fatal(err)
	}
	address := os.Getenv("HTTP_ADDRESS")
	if address == "" {
		address = "127.0.0.1:8080"
	}
	if err := app.Run(boot.Options{Address: address, HTTP: &boot.HTTPOptions{}}); err != nil {
		log.Fatal(err)
	}
}
