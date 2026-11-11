package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/NARUBROWN/spine"
	eventcontract "github.com/NARUBROWN/spine/contract/event"
	"github.com/NARUBROWN/spine/pkg/boot"
)

type recordingPublisher struct {
	mu     sync.Mutex
	events []string
	err    error
}

func (p *recordingPublisher) Publish(_ context.Context, message eventcontract.Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, message.EventName)
	return p.err
}

func (*recordingPublisher) Close(context.Context) error { return nil }

type testPublisherFactory struct {
	publisher *recordingPublisher
}

func (f testPublisherFactory) Capabilities() eventcontract.Capabilities {
	return eventcontract.Capabilities{ContractVersion: eventcontract.ContractVersion, Publish: true}
}

func (f testPublisherFactory) NewPublisher(context.Context) (eventcontract.Publisher, error) {
	return f.publisher, nil
}

func (testPublisherFactory) NewReader(context.Context, eventcontract.Subscription) (eventcontract.Reader, error) {
	return nil, errors.New("reader is unavailable")
}

func TestReferencePublishingOverHTTP(t *testing.T) {
	publishedEvents := []string{"order.created", "stock.created"}
	for _, test := range []struct {
		name          string
		withPublisher bool
		publishErr    error
		wantStatus    int
		wantEvents    []string
	}{
		{name: "발행기 없음", wantStatus: http.StatusServiceUnavailable},
		{name: "발행 성공", withPublisher: true, wantStatus: http.StatusOK, wantEvents: publishedEvents},
		{name: "발행 실패", withPublisher: true, publishErr: errors.New("broker unavailable"), wantStatus: http.StatusInternalServerError, wantEvents: publishedEvents},
	} {
		t.Run(test.name, func(t *testing.T) {
			publisher := &recordingPublisher{err: test.publishErr}
			var adapters []referenceEventAdapter
			if test.withPublisher {
				adapters = append(adapters, referenceEventAdapter{name: "test", factory: testPublisherFactory{publisher}})
			}
			app := newReferenceApp(adapters...)
			prepared, err := app.Prepare(t.Context(), boot.Options{HTTP: &boot.HTTPOptions{}})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := prepared.Close(context.Background()); err != nil {
					t.Error(err)
				}
			})
			handler, err := spine.NetHTTPHandler(prepared)
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(handler)
			t.Cleanup(server.Close)

			for _, path := range []string{"/orders/42", "/stocks/42"} {
				response, err := server.Client().Post(server.URL+path, "application/json", nil)
				if err != nil {
					t.Fatal(err)
				}
				response.Body.Close()
				if response.StatusCode != test.wantStatus {
					t.Errorf("POST %s 상태 = %d, 기대 상태 = %d", path, response.StatusCode, test.wantStatus)
				}
			}
			publisher.mu.Lock()
			defer publisher.mu.Unlock()
			if !slices.Equal(publisher.events, test.wantEvents) {
				t.Errorf("발행 호출 = %v, 기대 호출 = %v", publisher.events, test.wantEvents)
			}
		})
	}
}
