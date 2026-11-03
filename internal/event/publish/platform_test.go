package publish

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	eventadapter "github.com/NARUBROWN/spine/adapter/event"
)

type platformFactoryStub struct {
	create func(context.Context) (eventadapter.Publisher, error)
}

func (platformFactoryStub) Capabilities() eventadapter.Capabilities {
	return eventadapter.Capabilities{ContractVersion: eventadapter.ContractVersion, Publish: true}
}
func (platformFactoryStub) NewReader(context.Context, eventadapter.Subscription) (eventadapter.Reader, error) {
	panic("unexpected reader")
}
func (f platformFactoryStub) NewPublisher(ctx context.Context) (eventadapter.Publisher, error) {
	return f.create(ctx)
}

type nativePublisherStub struct {
	send     func(eventadapter.Message) error
	closes   atomic.Int32
	closeErr error
}

func (p *nativePublisherStub) Publish(_ context.Context, m eventadapter.Message) error {
	return p.send(m)
}
func (p *nativePublisherStub) Close(context.Context) error { p.closes.Add(1); return p.closeErr }

func TestPlatformPublisherRetiresAfterConcurrentCallsAndRecreatesOnNextPublish(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	cause := errors.New("transport failed")
	cleanupCause := errors.New("cleanup failed")
	old := &nativePublisherStub{closeErr: cleanupCause, send: func(m eventadapter.Message) error {
		if m.EventName == "hold" {
			close(started)
			<-release
			return nil
		}
		return cause
	}}
	next := &nativePublisherStub{send: func(eventadapter.Message) error { return nil }}
	var creates atomic.Int32
	p := NewPlatformPublisher(platformFactoryStub{create: func(context.Context) (eventadapter.Publisher, error) { creates.Add(1); return next, nil }}, old)
	held := make(chan error, 1)
	go func() { held <- p.Publish(context.Background(), eventadapter.Message{EventName: "hold"}) }()
	<-started
	if err := p.Publish(context.Background(), eventadapter.Message{EventName: "fail"}); !errors.Is(err, cause) {
		t.Fatal(err)
	}
	if old.closes.Load() != 0 {
		t.Fatal("진행 중인 발행보다 먼저 자원을 정리했습니다")
	}
	if err := p.Publish(context.Background(), eventadapter.Message{EventName: "next"}); err != nil {
		t.Fatal(err)
	}
	if creates.Load() != 1 {
		t.Fatal("다음 호출에서 발행기를 한 번 재생성해야 합니다")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := p.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	close(release)
	if err := <-held; err != nil {
		t.Fatal(err)
	}
	if err := p.Close(context.Background()); !errors.Is(err, cleanupCause) {
		t.Fatalf("이전 인스턴스의 정리 오류를 보존해야 합니다: %v", err)
	}
	if old.closes.Load() != 1 || next.closes.Load() != 1 {
		t.Fatal("각 발행기를 한 번만 정리해야 합니다")
	}
	if len(p.instances) != 0 {
		t.Fatal("정리한 native 인스턴스를 계속 보관하면 안 됩니다")
	}
}

func TestPlatformPublisherCreationWaitAndCloseHonorContexts(t *testing.T) {
	started := make(chan struct{})
	old := &nativePublisherStub{send: func(eventadapter.Message) error { return errors.New("failed") }}
	p := NewPlatformPublisher(platformFactoryStub{create: func(ctx context.Context) (eventadapter.Publisher, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}}, old)
	_ = p.Publish(context.Background(), eventadapter.Message{})
	done := make(chan error, 1)
	go func() { done <- p.Publish(context.Background(), eventadapter.Message{}) }()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := p.Publish(ctx, eventadapter.Message{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("생성 대기는 호출 context를 따라야 합니다: %v", err)
	}
	if err := p.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("종료가 진행 중인 생성을 취소하지 않았습니다: %v", err)
	}
}

func TestPlatformPublisherFactoryPanicDoesNotBlockClose(t *testing.T) {
	old := &nativePublisherStub{send: func(eventadapter.Message) error { return errors.New("failed") }}
	p := NewPlatformPublisher(platformFactoryStub{create: func(context.Context) (eventadapter.Publisher, error) { panic("factory failed") }}, old)
	_ = p.Publish(context.Background(), eventadapter.Message{})
	if err := p.Publish(context.Background(), eventadapter.Message{}); err == nil {
		t.Fatal("factory 패닉을 오류로 반환해야 합니다")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := p.Close(ctx); err != nil {
		t.Fatalf("실패한 생성이 Close를 막았습니다: %v", err)
	}
}
