// This example keeps one pending row in memory. A production outbox needs durable
// storage, transactional insertion, leasing and idempotent consumers.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/NARUBROWN/spine"
	rabbit "github.com/NARUBROWN/spine-rabbitmq-platform"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/constructor"
	"github.com/NARUBROWN/spine/pkg/event/publish"
)

type destinations []string

func (d destinations) PublishDestinations() []string { return d }

type poller struct {
	publisher publish.Publisher
	id        string
	at        time.Time
	sent      bool
}

func (p *poller) Run(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if !p.sent {
			callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			err := p.publisher.Event(callCtx, "orders.created", publish.Fields{"id": p.id}, publish.WithMessageID(p.id), publish.WithOccurredAt(p.at))
			cancel()
			if err == nil {
				p.sent = true
				log.Printf("confirmed outbox row %s", p.id)
			} else if ctx.Err() == nil {
				log.Printf("row %s remains pending: %v", p.id, err)
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
func run() error {
	address := os.Getenv("SPINE_DEMO_RABBITMQ_URL")
	if address == "" {
		return fmt.Errorf("set SPINE_DEMO_RABBITMQ_URL to a local AMQP test broker")
	}
	factory, err := rabbit.NewFactory(rabbit.Config{URL: address, Exchange: "spine-publish-only", Publish: true, PublishDestinations: destinations{"orders.created"}, AllowInsecureTransport: true})
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	workerCtx, cancelWorker := context.WithCancel(ctx)
	defer cancelWorker()
	done := make(chan struct{})
	var service *poller
	app := spine.New(spine.WithEventAdapter("rabbitmq", factory), spine.WithShutdownHook(func(shutdown context.Context, p *poller) error {
		cancelWorker()
		select {
		case <-done:
			return nil
		case <-shutdown.Done():
			return shutdown.Err()
		}
	}))
	app.Constructor(constructor.Custom(func(p publish.Publisher) *poller {
		service = &poller{publisher: p, id: "outbox-example-42", at: time.Now().UTC()}
		return service
	}))
	prepared, err := app.Prepare(ctx, boot.Options{})
	if err != nil {
		return err
	}
	// Start explicitly after preparation; constructors only assemble dependencies.
	go func() {
		defer close(done)
		if err := service.Run(workerCtx); err != nil {
			log.Print(err)
		}
	}()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return prepared.Close(shutdown)
}
func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
