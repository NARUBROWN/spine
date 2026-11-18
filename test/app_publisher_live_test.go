package test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NARUBROWN/spine"
	rabbit "github.com/NARUBROWN/spine-rabbitmq-platform"
	event "github.com/NARUBROWN/spine/contract/event"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/event/publish"
	amqp "github.com/rabbitmq/amqp091-go"
)

type countedLiveFactory struct {
	event.Factory
	builds atomic.Int32
}

func (f *countedLiveFactory) NewPublisher(ctx context.Context) (event.Publisher, error) {
	f.builds.Add(1)
	return f.Factory.NewPublisher(ctx)
}

// The proxy forwards actual AMQP traffic and can break only this test's publisher connection.
func publisherWireProxy(t *testing.T, address string) (string, func()) {
	t.Helper()
	parsed, err := url.Parse(address)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme != "amqp" {
		t.Fatal("wire-drop test requires an isolated plaintext AMQP test broker")
	}
	target := parsed.Host
	if parsed.Port() == "" {
		target = net.JoinHostPort(parsed.Hostname(), "5672")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var wires []net.Conn
	var wg sync.WaitGroup
	drop := func() {
		mu.Lock()
		defer mu.Unlock()
		for _, wire := range wires {
			wire.Close()
		}
		wires = nil
	}
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		for {
			client, err := listener.Accept()
			if err != nil {
				return
			}
			upstream, err := net.DialTimeout("tcp", target, time.Second)
			if err != nil {
				client.Close()
				continue
			}
			mu.Lock()
			wires = append(wires, client, upstream)
			mu.Unlock()
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer client.Close()
				defer upstream.Close()
				copied := make(chan struct{})
				go func() { io.Copy(upstream, client); upstream.Close(); close(copied) }()
				io.Copy(client, upstream)
				client.Close()
				<-copied
			}()
		}
	}()
	t.Cleanup(func() { listener.Close(); <-stopped; drop(); wg.Wait() })
	parsed.Host = listener.Addr().String()
	return parsed.String(), drop
}

func TestAppPublisherLive_RabbitMQConfirmUnroutableOutboxRetryAndReconnect(t *testing.T) {
	address := os.Getenv("SPINE_TEST_RABBITMQ_URL")
	if address == "" {
		t.Skip("SPINE_TEST_RABBITMQ_URL is not configured")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	connection, err := amqp.Dial(address)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	channel, err := connection.Channel()
	if err != nil {
		t.Fatal(err)
	}
	defer channel.Close()
	name := fmt.Sprintf("spine-publisher-%d", time.Now().UnixNano())
	defer channel.ExchangeDelete(name, false, false)
	defer channel.QueueDelete(name, false, false, false)
	proxyURL, drop := publisherWireProxy(t, address)
	native, err := rabbit.NewFactory(rabbit.Config{URL: proxyURL, Exchange: name, Publish: true, PublishDestinations: testDestinations{"orders"}, AllowInsecureTransport: true, OperationTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	factory := &countedLiveFactory{Factory: native}
	capture := &publisherCapture{}
	app := spine.New(spine.WithEventAdapter("rabbitmq", factory))
	app.RegisterTransport(capture)
	prepared, err := app.Prepare(ctx, boot.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close(context.Background())
	at := time.Unix(1700000000, 0).UTC()
	options := []publish.Option{publish.WithMessageID("outbox-42"), publish.WithOccurredAt(at), publish.WithMetadata(map[string]string{"trace": "stable"})}
	published := false
	send := func() error {
		err := capture.publisher.Event(ctx, "orders", publish.Fields{"id": "outbox-42"}, options...)
		if err == nil {
			published = true
		}
		return err
	}
	if err := send(); !errors.Is(err, rabbit.ErrUnroutable) || published {
		t.Fatalf("unroutable=%v marked=%t", err, published)
	}
	if factory.builds.Load() != 1 {
		t.Fatal("failed call retried implicitly")
	}
	if _, err := channel.QueueDeclare(name, true, false, false, false, nil); err != nil {
		t.Fatal(err)
	}
	if err := channel.QueueBind(name, "orders", name, false, nil); err != nil {
		t.Fatal(err)
	}
	if err := send(); err != nil || !published {
		t.Fatalf("retry=%v marked=%t", err, published)
	}
	verify := func() {
		t.Helper()
		delivery, ok, err := channel.Get(name, true)
		if err != nil || !ok {
			t.Fatalf("confirmed message missing: %t %v", ok, err)
		}
		if string(delivery.Body) != `{"id":"outbox-42"}` || delivery.Headers["message_id"] != "outbox-42" || delivery.Headers["trace"] != "stable" || !delivery.Timestamp.Equal(at) {
			t.Fatalf("outbox metadata changed: %+v", delivery)
		}
	}
	verify()
	builds := factory.builds.Load()
	drop()
	published = false
	if err := send(); err == nil || published {
		t.Fatalf("broken connection accepted: %v", err)
	}
	if factory.builds.Load() != builds {
		t.Fatal("failed transmission was retried implicitly")
	}
	if err := send(); err != nil {
		t.Fatalf("reconnect=%v", err)
	}
	if factory.builds.Load() != builds+1 {
		t.Fatal("next call did not recreate the connection")
	}
	verify()
	if err := capture.publisher.Event(ctx, "undeclared", nil); !errors.Is(err, event.ErrInvalidAdapter) {
		t.Fatalf("undeclared destination=%v", err)
	}
	if err := prepared.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := send(); !errors.Is(err, event.ErrInvalidState) {
		t.Fatalf("closed call=%v", err)
	}
}
