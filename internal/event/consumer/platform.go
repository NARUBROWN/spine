package consumer

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	eventadapter "github.com/NARUBROWN/spine/contract/event"
	"github.com/NARUBROWN/spine/pkg/boot"
)

// PlatformFactory는 공개 전송 계약을 커널의 공통 실행 런타임에 연결합니다.
type PlatformFactory struct {
	factory eventadapter.Factory
	options boot.EventOptions
}

func NewPlatformFactory(factory eventadapter.Factory, options boot.EventOptions) *PlatformFactory {
	if options.SettlementTimeout == 0 {
		options.SettlementTimeout = 5 * time.Second
	}
	return &PlatformFactory{factory: factory, options: options}
}

func (f *PlatformFactory) Build(reg Registration) (Reader, error) {
	return f.BuildContext(context.Background(), reg)
}

func (f *PlatformFactory) BuildContext(ctx context.Context, reg Registration) (Reader, error) {
	reader, err := f.factory.NewReader(ctx, eventadapter.Subscription{Topic: reg.Topic})
	if err != nil {
		if !nilPlatformValue(reader) {
			err = errors.Join(err, reader.Close(context.Background()))
		}
		return nil, err
	}
	if nilPlatformValue(reader) {
		return nil, fmt.Errorf("%w: factory returned a nil reader", eventadapter.ErrInvalidAdapter)
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.Join(err, reader.Close(context.Background()))
	}
	return &platformReader{native: reader, topic: reg.Topic, settlementTimeout: f.options.SettlementTimeout}, nil
}

func (f *PlatformFactory) ConsumerRetryPolicy() TransportRetryPolicy {
	p := f.options.ConsumerRetry
	return NewTransportRetryPolicy(p.InitialDelay, p.MaxDelay, p.Multiplier, p.Jitter, p.MaxAttempts)
}

type platformReader struct {
	native            eventadapter.Reader
	topic             string
	settlementTimeout time.Duration
}

func (r *platformReader) Read(ctx context.Context) (*Message, error) {
	delivery, err := r.native.Read(ctx)
	if err != nil {
		return nil, err
	}
	if nilPlatformValue(delivery) {
		return nil, fmt.Errorf("%w: reader returned a nil delivery", eventadapter.ErrInvalidAdapter)
	}
	envelope := delivery.Message()
	// 구독 키로 핸들러를 선택합니다. 생산자가 보낸 이벤트 이름은 재라우팅에 사용하지 않습니다.
	message := &Message{EventName: r.topic, Payload: envelope.Payload, Metadata: envelope.Metadata}
	settle := func(success bool) error {
		// 종료 신호가 도착해도 실행을 마친 전달은 제한 시간 안에서 확인을 끝냅니다.
		settlementCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), r.settlementTimeout)
		defer cancel()
		if success {
			return delivery.Ack(settlementCtx)
		}
		return delivery.Nack(settlementCtx)
	}
	message.SetAckHandler(func() error { return settle(true) })
	message.SetNackHandler(func() error { return settle(false) })
	return message, nil
}

func (r *platformReader) Close() error { return r.native.Close(context.Background()) }

func nilPlatformValue(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	}
	return false
}
