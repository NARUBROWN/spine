package bootstrap

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	adapter "github.com/NARUBROWN/spine/contract/http"
	wsadapter "github.com/NARUBROWN/spine/contract/websocket"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/ws"
	"github.com/NARUBROWN/spine/pkg/boot"
)

type testWSFactory struct {
	version  string
	newCalls int
	options  wsadapter.Options
	instance wsadapter.Adapter
	err      error
}

func (f *testWSFactory) Capabilities() wsadapter.Capabilities {
	return wsadapter.Capabilities{ContractVersion: f.version}
}
func (f *testWSFactory) New(options wsadapter.Options) (wsadapter.Adapter, error) {
	f.newCalls++
	f.options = options
	return f.instance, f.err
}

type testWSAdapter struct{}

func (*testWSAdapter) Upgrade(http.ResponseWriter, *http.Request) (wsadapter.Connection, error) {
	panic("Prepare must not upgrade")
}

type wsFactoryController struct{}

func (*wsFactoryController) Handle() {}

func wsFactoryConfig(t *testing.T, factory wsadapter.Factory) Config {
	t.Helper()
	registry := ws.NewRegistry()
	if err := registry.Register("/ws", (*wsFactoryController).Handle); err != nil {
		t.Fatal(err)
	}
	httpFactory := &upgradeConfigurableFactory{configurableFactory: configurableFactory{capabilities: adapter.Capabilities{ContractVersion: adapter.ContractVersion, NetHTTPUpgrade: true}, newAdapter: testHTTPFactory{}.New}}
	return Config{HTTP: &boot.HTTPOptions{}, HTTPAdapterFactory: httpFactory, WebSocketAdapterFactory: factory, WebSocketRegistry: registry, Constructors: []any{func() *wsFactoryController { return &wsFactoryController{} }}}
}

func TestWSFactoryPreflightBeforeInitialization(t *testing.T) {
	var typedNil *testWSFactory
	for _, factory := range []wsadapter.Factory{nil, typedNil, &testWSFactory{version: "unsupported"}} {
		transport := &testTransport{}
		config := wsFactoryConfig(t, factory)
		config.CustomTransports = []core.CustomTransport{transport}
		if err := Run(config); !errors.Is(err, wsadapter.ErrInvalidAdapter) || transport.initCalls.Load() != 0 {
			t.Fatalf("WS 사전 검사 실패 시 초기화하면 안 됩니다: %v", err)
		}
	}
}

func TestWSFactoryPrepareReceivesOnlyNormalizedConnectionOptions(t *testing.T) {
	factory := &testWSFactory{version: wsadapter.ContractVersion, instance: &testWSAdapter{}}
	config := wsFactoryConfig(t, factory)
	config.HTTP.WebSocket.AllowedOrigins = []string{"https://example.test"}
	if err := Validate(config); err != nil || factory.newCalls != 0 {
		t.Fatalf("검증에서 factory.New를 실행하면 안 됩니다: %v", err)
	}
	prepared, err := Prepare(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close(context.Background())
	if factory.newCalls != 1 || factory.options != (wsadapter.Options{HandshakeTimeout: 10 * time.Second, ReadTimeout: 60 * time.Second, MaxMessageBytes: 1 << 20}) {
		t.Fatalf("연결 기본값을 한 번 전달해야 합니다: %+v", factory)
	}
}

func TestWSFactoryFailureRollsBackInitializedResources(t *testing.T) {
	want := errors.New("WS adapter construction failed")
	var typedNil *testWSAdapter
	for _, tc := range []struct {
		instance wsadapter.Adapter
		err      error
	}{{nil, want}, {nil, nil}, {typedNil, nil}} {
		factory := &testWSFactory{version: wsadapter.ContractVersion, instance: tc.instance, err: tc.err}
		transport := &testTransport{}
		config := wsFactoryConfig(t, factory)
		config.CustomTransports = []core.CustomTransport{transport}
		prepared, err := Prepare(context.Background(), config)
		expected := tc.err
		if expected == nil {
			expected = wsadapter.ErrInvalidAdapter
		}
		if prepared != nil || !errors.Is(err, expected) || transport.stopCalls.Load() != 1 {
			t.Fatalf("준비 실패 시 초기화 자원을 회수해야 합니다: %v", err)
		}
	}
}

func TestHTTPOnlyDoesNotRequireOrCreateWSAdapter(t *testing.T) {
	factory := &testWSFactory{version: "unused"}
	prepared, err := Prepare(context.Background(), Config{HTTP: &boot.HTTPOptions{}, HTTPAdapterFactory: testHTTPFactory{}, WebSocketAdapterFactory: factory})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close(context.Background())
	if factory.newCalls != 0 {
		t.Fatal("WS 경로가 없는 앱에서 WS 구현을 생성하면 안 됩니다")
	}
}

type upgradeConfigurableFactory struct{ configurableFactory }

func (f *upgradeConfigurableFactory) NewWithNetHTTPUpgrades(c adapter.Config, _ []adapter.NetHTTPUpgradeRoute) (adapter.HTTPAdapter, error) {
	return f.New(c)
}
