package eventadapter_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/NARUBROWN/spine/adapter/event"
)

// 외부 구현이 internal 타입이나 도메인 이벤트 없이 계약 전체를 만족할 수 있는지 확인합니다.
var (
	_ eventadapter.Factory   = (*testFactory)(nil)
	_ eventadapter.Reader    = (*testReader)(nil)
	_ eventadapter.Delivery  = (*testDelivery)(nil)
	_ eventadapter.Publisher = (*testPublisher)(nil)
)

type testFactory struct {
	capabilities      eventadapter.Capabilities
	capabilitiesCalls int
}

func (f *testFactory) Capabilities() eventadapter.Capabilities {
	f.capabilitiesCalls++
	return f.capabilities
}

func (*testFactory) NewReader(context.Context, eventadapter.Subscription) (eventadapter.Reader, error) {
	panic("validation must not create a reader")
}

func (*testFactory) NewPublisher(context.Context) (eventadapter.Publisher, error) {
	panic("validation must not create a publisher")
}

// 아래 fixture는 메서드 시그니처만 검사하며 브로커 동작을 대신하지 않습니다.
type testReader struct{}
type testDelivery struct{}
type testPublisher struct{}

func (*testReader) Read(context.Context) (eventadapter.Delivery, error) {
	panic("signature fixture")
}
func (*testReader) Close(context.Context) error { panic("signature fixture") }
func (*testDelivery) Message() eventadapter.Message {
	panic("signature fixture")
}
func (*testDelivery) Ack(context.Context) error  { panic("signature fixture") }
func (*testDelivery) Nack(context.Context) error { panic("signature fixture") }
func (*testPublisher) Publish(context.Context, eventadapter.Message) error {
	panic("signature fixture")
}
func (*testPublisher) Close(context.Context) error { panic("signature fixture") }

func TestValidateFactory(t *testing.T) {
	consume := eventadapter.Capabilities{ContractVersion: eventadapter.ContractVersion, Consume: true}
	publish := eventadapter.Capabilities{ContractVersion: eventadapter.ContractVersion, Publish: true}
	both := eventadapter.Capabilities{ContractVersion: eventadapter.ContractVersion, Consume: true, Publish: true}

	tests := []struct {
		name         string
		capabilities eventadapter.Capabilities
		required     eventadapter.Requirements
		wantErr      error
		details      []string
	}{
		{name: "consume_only", capabilities: consume, required: eventadapter.Requirements{Consume: true}},
		{name: "publish_only", capabilities: publish, required: eventadapter.Requirements{Publish: true}},
		{name: "both_roles", capabilities: both, required: eventadapter.Requirements{Consume: true, Publish: true}},
		{name: "extra_roles_allowed", capabilities: both, required: eventadapter.Requirements{Consume: true}},
		{name: "no_requirements", capabilities: consume},
		{name: "missing_consume", capabilities: publish, required: eventadapter.Requirements{Consume: true}, wantErr: eventadapter.ErrUnsupportedCapability, details: []string{"Consume"}},
		{name: "missing_publish", capabilities: consume, required: eventadapter.Requirements{Publish: true}, wantErr: eventadapter.ErrUnsupportedCapability, details: []string{"Publish"}},
		{name: "missing_version", capabilities: eventadapter.Capabilities{Consume: true}, wantErr: eventadapter.ErrInvalidAdapter, details: []string{eventadapter.ContractVersion}},
		{name: "different_version", capabilities: eventadapter.Capabilities{ContractVersion: "spine.event-adapter/v0", Consume: true}, wantErr: eventadapter.ErrInvalidAdapter, details: []string{"spine.event-adapter/v0", eventadapter.ContractVersion}},
		{name: "HTTP_version_rejected", capabilities: eventadapter.Capabilities{ContractVersion: "spine.http-adapter/v2", Consume: true}, wantErr: eventadapter.ErrInvalidAdapter},
		{name: "version_checked_before_roles", capabilities: eventadapter.Capabilities{ContractVersion: "spine.event-adapter/v0"}, required: eventadapter.Requirements{Consume: true}, wantErr: eventadapter.ErrInvalidAdapter},
		{name: "no_supported_roles", capabilities: eventadapter.Capabilities{ContractVersion: eventadapter.ContractVersion}, required: eventadapter.Requirements{Publish: true}, wantErr: eventadapter.ErrInvalidAdapter, details: []string{"Consume", "Publish"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory := &testFactory{capabilities: tt.capabilities}
			err := eventadapter.ValidateFactory(factory, tt.required)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("지원되는 구성은 검사를 통과해야 합니다: %v", err)
				}
			} else {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("오류 종류 = %v, 기대값 = %v", err, tt.wantErr)
				}
				if !errors.Is(fmt.Errorf("event setup: %w", err), tt.wantErr) {
					t.Fatalf("상위 오류로 감싸도 원래 오류 종류를 보존해야 합니다: %v", err)
				}
				for _, detail := range tt.details {
					if !strings.Contains(err.Error(), detail) {
						t.Fatalf("진단 정보 %q가 없습니다: %v", detail, err)
					}
				}
			}
			if factory.capabilitiesCalls != 1 {
				t.Fatalf("기능 선언 조회 횟수 = %d, 기대값 = 1", factory.capabilitiesCalls)
			}
		})
	}
}

func TestValidateFactoryRejectsNilBeforeCallingMethods(t *testing.T) {
	tests := []struct {
		name    string
		factory eventadapter.Factory
	}{
		{name: "interface"},
		{name: "pointer", factory: (*testFactory)(nil)},
		{name: "function", factory: functionFactory(nil)},
		{name: "map", factory: mapFactory(nil)},
		{name: "slice", factory: sliceFactory(nil)},
		{name: "channel", factory: channelFactory(nil)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := eventadapter.ValidateFactory(tt.factory, eventadapter.Requirements{})
			if !errors.Is(err, eventadapter.ErrInvalidAdapter) || !strings.Contains(err.Error(), "nil") {
				t.Fatalf("타입이 있는 nil도 메서드를 호출하기 전에 거부해야 합니다: %v", err)
			}
		})
	}
}

func TestValidateFactoryAcceptsNonNilFactoryKinds(t *testing.T) {
	tests := []struct {
		name    string
		factory eventadapter.Factory
	}{
		{name: "value", factory: valueFactory{}},
		{name: "function", factory: functionFactory(func() {})},
		{name: "map", factory: mapFactory{}},
		{name: "slice", factory: sliceFactory{}},
		{name: "channel", factory: make(channelFactory)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := eventadapter.ValidateFactory(tt.factory, eventadapter.Requirements{Consume: true, Publish: true}); err != nil {
				t.Fatalf("nil이 아닌 factory의 종류를 제한하면 안 됩니다: %v", err)
			}
		})
	}
}

type valueFactory struct{}
type functionFactory func()
type mapFactory map[string]string
type sliceFactory []string
type channelFactory chan struct{}

func fullCapabilities() eventadapter.Capabilities {
	return eventadapter.Capabilities{ContractVersion: eventadapter.ContractVersion, Consume: true, Publish: true}
}

func (valueFactory) Capabilities() eventadapter.Capabilities    { return fullCapabilities() }
func (functionFactory) Capabilities() eventadapter.Capabilities { return fullCapabilities() }
func (mapFactory) Capabilities() eventadapter.Capabilities      { return fullCapabilities() }
func (sliceFactory) Capabilities() eventadapter.Capabilities    { return fullCapabilities() }
func (channelFactory) Capabilities() eventadapter.Capabilities  { return fullCapabilities() }

func (valueFactory) NewReader(context.Context, eventadapter.Subscription) (eventadapter.Reader, error) {
	panic("validation must not create a reader")
}
func (functionFactory) NewReader(context.Context, eventadapter.Subscription) (eventadapter.Reader, error) {
	panic("validation must not create a reader")
}
func (mapFactory) NewReader(context.Context, eventadapter.Subscription) (eventadapter.Reader, error) {
	panic("validation must not create a reader")
}
func (sliceFactory) NewReader(context.Context, eventadapter.Subscription) (eventadapter.Reader, error) {
	panic("validation must not create a reader")
}
func (channelFactory) NewReader(context.Context, eventadapter.Subscription) (eventadapter.Reader, error) {
	panic("validation must not create a reader")
}

func (valueFactory) NewPublisher(context.Context) (eventadapter.Publisher, error) {
	panic("validation must not create a publisher")
}
func (functionFactory) NewPublisher(context.Context) (eventadapter.Publisher, error) {
	panic("validation must not create a publisher")
}
func (mapFactory) NewPublisher(context.Context) (eventadapter.Publisher, error) {
	panic("validation must not create a publisher")
}
func (sliceFactory) NewPublisher(context.Context) (eventadapter.Publisher, error) {
	panic("validation must not create a publisher")
}
func (channelFactory) NewPublisher(context.Context) (eventadapter.Publisher, error) {
	panic("validation must not create a publisher")
}

func ExampleValidateFactory() {
	factory := &testFactory{capabilities: eventadapter.Capabilities{
		ContractVersion: eventadapter.ContractVersion,
		Consume:         true,
	}}
	err := eventadapter.ValidateFactory(factory, eventadapter.Requirements{Consume: true})
	fmt.Println(err)
	// Output: <nil>
}
