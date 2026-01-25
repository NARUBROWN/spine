package main

import (
	"time"

	"github.com/NARUBROWN/spine"
	"github.com/NARUBROWN/spine/interceptor/cors"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/route"
)

func main() {
	app := spine.New()

	// 생성자 등록
	app.Constructor(
		NewUserController,
		NewOrderConsumer,
	)

	// 라우트 등록, 라우터 단위 인터셉터
	app.Route(
		"GET",
		"/users/:id",
		(*UserController).GetUser,
		route.WithInterceptors(&LoggingInterceptor{}),
	)

	app.Route(
		"POST",
		"/users",
		(*UserController).CreateUser,
	)

	app.Route(
		"GET",
		"/users",
		(*UserController).GetUserQuery,
	)

	app.Route(
		"POST",
		"/upload",
		(*UserController).Upload,
	)

	app.Route(
		"POST",
		"/orders/:orderId",
		(*UserController).CreateOrder,
	)

	app.Route(
		"POST",
		"/stocks/:stockId",
		(*UserController).CreateStock,
	)

	app.Interceptor(
		cors.New(cors.Config{
			AllowOrigins: []string{"*"},
			AllowMethods: []string{"GET", "POST", "OPTIONS"},
			AllowHeaders: []string{"Content-Type"},
		}),
	)

	app.Consumers().Register(
		"order.created",
		(*OrderConsumer).OnCreatedKafka,
	)

	app.Consumers().Register(
		"stock.created",
		(*OrderConsumer).OnCreatedRabbitMQ,
	)

	// EnableGracefulShutdown & ShutdownTimeout은 선택사항입니다.
	app.Run(boot.Options{
		Address:                ":8080",
		EnableGracefulShutdown: true,
		ShutdownTimeout:        10 * time.Second,
		Kafka: &boot.KafkaOptions{
			Brokers: []string{"localhost:9092"},
			Read: &boot.KafkaReadOptions{
				GroupID: "spine-demo-consumer",
			},
			Write: &boot.KafkaWriteOptions{
				TopicPrefix: "",
			},
		},
		RabbitMQ: &boot.RabbitMqOptions{
			URL: "amqp://guest:guest@localhost:5672/",
			Read: &boot.RabbitMqReadOptions{
				Queue:      "stock.created",
				Exchange:   "stock-exchange",
				RoutingKey: "stock.created",
			},
			Write: &boot.RabbitMqWriteOptions{
				Exchange:   "stock-exchange",
				RoutingKey: "stock.created",
			},
		},
	})
}
