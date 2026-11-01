package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NARUBROWN/spine"
	"github.com/NARUBROWN/spine/cmd/demo/application"
	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/gorilla/websocket"
)

const (
	e2eHelperEnv        = "SPINE_DEMO_E2E_HELPER"
	e2eAddressEnv       = "SPINE_DEMO_ADDRESS"
	e2eSignalOnMountEnv = "SPINE_DEMO_E2E_SIGNAL_ON_MOUNT"
	e2eSignalOnInitEnv  = "SPINE_DEMO_E2E_SIGNAL_ON_INIT"
)

type e2eSignalOnInitTransport struct {
	stopped  chan struct{}
	stopOnce sync.Once
}

func (t *e2eSignalOnInitTransport) Init(core.Container) error {
	process, err := os.FindProcess(os.Getpid())
	if err != nil {
		return err
	}
	return process.Signal(os.Interrupt)
}

func (t *e2eSignalOnInitTransport) Start() error {
	<-t.stopped
	return nil
}

func (t *e2eSignalOnInitTransport) Stop(context.Context) error {
	t.stopOnce.Do(func() { close(t.stopped) })
	return nil
}

// TestDemoE2EHelperProcess는 자식 프로세스에서 실제 demo의 run을 실행합니다.
// 테스트 실행기에는 신호를 보내지 않고 HTTP/WS와 신호 기반 종료를 검사합니다.
func TestDemoE2EHelperProcess(t *testing.T) {
	if os.Getenv(e2eHelperEnv) != "1" {
		return
	}

	if os.Getenv(e2eSignalOnInitEnv) == "1" {
		transport := &e2eSignalOnInitTransport{stopped: make(chan struct{})}
		app := spine.New()
		app.RegisterTransport(transport)
		if err := app.Run(boot.Options{ShutdownTimeout: 3 * time.Second}); err != nil {
			t.Fatalf("run custom transport E2E helper app: %v", err)
		}
		select {
		case <-transport.stopped:
		default:
			t.Fatal("custom transport was not stopped after startup signal")
		}
		return
	}

	address := os.Getenv(e2eAddressEnv)
	if address == "" {
		t.Fatal("E2E helper address is empty")
	}

	if os.Getenv(e2eSignalOnMountEnv) == "1" {
		app, options, err := application.BuildApplication()
		if err != nil {
			t.Fatal(err)
		}
		app.Transport(func(any) {
			process, err := os.FindProcess(os.Getpid())
			if err == nil {
				_ = process.Signal(os.Interrupt)
			}
		})
		if err := app.Validate(options); err != nil {
			t.Fatal(err)
		}
		if err := app.Run(options); err != nil {
			t.Fatalf("run startup signal helper: %v", err)
		}
		return
	}

	if err := run(); err != nil {
		t.Fatalf("run demo: %v", err)
	}
}

func TestDemoE2E_HTTPWebSocketAndShutdown(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("graceful signal verification requires Unix process signals")
	}

	address := reserveTCPAddress(t)
	var processOutput e2eOutput
	cmd := exec.Command(os.Args[0], "-test.run=^TestDemoE2EHelperProcess$")
	cmd.Env = demoE2EEnv(e2eHelperEnv+"=1", e2eAddressEnv+"="+address)
	cmd.Stdout = &processOutput
	cmd.Stderr = &processOutput
	if err := cmd.Start(); err != nil {
		t.Fatalf("start E2E helper process: %v", err)
	}

	exited := make(chan struct{})
	var processErr error
	go func() {
		processErr = cmd.Wait()
		close(exited)
	}()
	processStopped := false
	t.Cleanup(func() {
		if processStopped {
			return
		}
		_ = cmd.Process.Kill()
		<-exited
	})

	baseURL := "http://" + address
	waitForHTTP(t, baseURL+"/health", exited, &processErr, &processOutput)

	client := &http.Client{
		Timeout:   2 * time.Second,
		Transport: http.DefaultTransport.(*http.Transport).Clone(),
	}
	defer client.CloseIdleConnections()
	for _, tt := range []struct {
		name        string
		method      string
		path        string
		contentType string
		body        string
		status      int
		wantJSON    string
	}{
		{name: "health", method: "GET", path: "/health", status: 200, wantJSON: `{"status":"ok"}`},
		{name: "path_binding", method: "GET", path: "/users/43", status: 200, wantJSON: `{"id":43}`},
		{name: "JSON_binding", method: "POST", path: "/echo", contentType: "application/json", body: `{"message":"E2E 확인","count":2}`, status: 200, wantJSON: `{"message":"E2E 확인","count":2}`},
		{name: "malformed_JSON", method: "POST", path: "/echo", contentType: "application/json", body: `{`, status: 400},
		{name: "unsupported_type", method: "POST", path: "/echo", contentType: "text/plain", body: "hello", status: 415},
		{name: "missing_type", method: "POST", path: "/echo", body: `{"message":"hello"}`, status: 415},
		{name: "body_limit", method: "POST", path: "/echo", contentType: "application/json", body: `{"message":"` + strings.Repeat("x", 1024) + `"}`, status: 413},
		{name: "missing_route", method: "GET", path: "/missing", status: 404},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assertDemoHTTP(t, client, tt.method, baseURL+tt.path, tt.contentType, tt.body, tt.status, tt.wantJSON)
		})
	}

	wsURL := "ws://" + address + "/ws/echo"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()
	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := conn.SetWriteDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"message":"real-network-e2e"}`)
	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		_ = conn.Close()
		t.Fatalf("write websocket message: %v", err)
	}
	messageType, echoed, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read websocket message: %v", err)
	}
	if messageType != websocket.TextMessage || string(echoed) != string(payload) {
		t.Fatalf("unexpected websocket echo: type=%d payload=%q", messageType, echoed)
	}

	// 테스트 클라이언트가 준비한 미사용 연결까지 닫아 서버의 정상 종료를 방해하지 않게 합니다.
	client.CloseIdleConnections()

	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("signal E2E helper process: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := conn.ReadMessage(); !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
		t.Fatalf("열린 WS 연결이 서버 정상 종료로 닫혀야 합니다: %v", err)
	}
	select {
	case <-exited:
		processStopped = true
		if processErr != nil {
			t.Fatalf("E2E helper did not shut down cleanly: %v\n%s", processErr, processOutput.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("E2E helper graceful shutdown timed out")
	}
}

func TestDemoE2E_ShutdownSignalDuringStartup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("graceful signal verification requires Unix process signals")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	address := reserveTCPAddress(t)
	var processOutput e2eOutput
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDemoE2EHelperProcess$")
	cmd.Env = demoE2EEnv(
		e2eHelperEnv+"=1",
		e2eAddressEnv+"="+address,
		e2eSignalOnMountEnv+"=1",
	)
	cmd.Stdout = &processOutput
	cmd.Stderr = &processOutput

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			t.Fatalf("startup signal was not handled before timeout: %v\n%s", ctx.Err(), processOutput.String())
		}
		t.Fatalf("startup signal did not shut down cleanly: %v\n%s", err, processOutput.String())
	}
}

func TestDemoE2E_CustomTransportShutdownSignalDuringInit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("graceful signal verification requires Unix process signals")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var processOutput e2eOutput
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDemoE2EHelperProcess$")
	cmd.Env = demoE2EEnv(e2eHelperEnv+"=1", e2eSignalOnInitEnv+"=1")
	cmd.Stdout = &processOutput
	cmd.Stderr = &processOutput

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			t.Fatalf("custom transport startup signal was not handled before timeout: %v\n%s", ctx.Err(), processOutput.String())
		}
		t.Fatalf("custom transport startup signal did not shut down cleanly: %v\n%s", err, processOutput.String())
	}
}

func assertDemoHTTP(t *testing.T, client *http.Client, method, url, contentType, body string, status int, wantJSON string) {
	t.Helper()
	request, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 4096))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != status {
		t.Fatalf("상태 = %d, 기대값 = %d, 응답 = %s", response.StatusCode, status, data)
	}
	if wantJSON == "" {
		return
	}
	if !strings.HasPrefix(response.Header.Get("Content-Type"), "application/json") {
		t.Fatalf("JSON Content-Type이 아닙니다: %q", response.Header.Get("Content-Type"))
	}
	var got, want any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(wantJSON), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("응답 = %s, 기대값 = %s", data, wantJSON)
	}
}

// demoE2EEnv는 개발자 환경이 테스트 앱의 주소나 동작을 바꾸지 않도록 격리합니다.
func demoE2EEnv(values ...string) []string {
	var env []string
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "SPINE_DEMO_") && !strings.HasPrefix(value, "SPINE_PORT=") {
			env = append(env, value)
		}
	}
	return append(env, values...)
}

type e2eOutput struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (o *e2eOutput) Write(data []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buffer.Write(data)
}

func (o *e2eOutput) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buffer.String()
}

func reserveTCPAddress(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve TCP address: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release reserved TCP address: %v", err)
	}
	return address
}

func waitForHTTP(t *testing.T, url string, exited <-chan struct{}, processErr *error, output fmt.Stringer) {
	t.Helper()

	client := &http.Client{
		Timeout:   250 * time.Millisecond,
		Transport: http.DefaultTransport.(*http.Transport).Clone(),
	}
	defer client.CloseIdleConnections()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-exited:
			t.Fatalf("E2E helper exited before readiness: %v\n%s", *processErr, output.String())
		default:
		}

		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}

	t.Fatalf("E2E helper readiness timed out at %s", strings.TrimSpace(url))
}
