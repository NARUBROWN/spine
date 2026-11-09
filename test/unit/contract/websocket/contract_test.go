package wscontract_test

import (
	"errors"
	"testing"
	"time"

	wsadapter "github.com/NARUBROWN/spine/contract/websocket"
)

type factory struct {
	version  string
	newCalls int
}

func (f *factory) Capabilities() wsadapter.Capabilities {
	return wsadapter.Capabilities{ContractVersion: f.version}
}
func (f *factory) New(wsadapter.Options) (wsadapter.Adapter, error) {
	f.newCalls++
	panic("validation must not create an adapter")
}

func TestValidateFactory(t *testing.T) {
	var typedNil *factory
	for _, f := range []wsadapter.Factory{nil, typedNil, &factory{}, &factory{version: "spine.websocket-adapter/v0"}} {
		if err := wsadapter.ValidateFactory(f); !errors.Is(err, wsadapter.ErrInvalidAdapter) {
			t.Fatalf("잘못된 factory를 거부해야 합니다: %v", err)
		}
	}
	f := &factory{version: wsadapter.ContractVersion}
	if err := wsadapter.ValidateFactory(f); err != nil || f.newCalls != 0 {
		t.Fatalf("사전 검사에서 New를 호출하면 안 됩니다: %v", err)
	}
}

func TestOptionsRequireNormalizedLimits(t *testing.T) {
	valid := wsadapter.Options{HandshakeTimeout: time.Second, ReadTimeout: time.Second, MaxMessageBytes: 1024}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, options := range []wsadapter.Options{{}, {HandshakeTimeout: -1, ReadTimeout: time.Second, MaxMessageBytes: 1}, {HandshakeTimeout: time.Second, MaxMessageBytes: 1}, {HandshakeTimeout: time.Second, ReadTimeout: time.Second}} {
		if err := options.Validate(); !errors.Is(err, wsadapter.ErrInvalidAdapter) {
			t.Fatalf("정규화되지 않은 설정을 거부해야 합니다: %v", err)
		}
	}
}
