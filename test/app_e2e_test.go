package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/NARUBROWN/spine"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/httpx"
	pkgws "github.com/NARUBROWN/spine/pkg/ws"
	"github.com/gorilla/websocket"
)

const (
	e2eHelperEnv        = "SPINE_E2E_HELPER"
	e2eAddressEnv       = "SPINE_E2E_ADDRESS"
	e2eSignalOnMountEnv = "SPINE_E2E_SIGNAL_ON_MOUNT"
)

type e2eController struct{}

func (c *e2eController) Health() httpx.Response[string] {
	return httpx.Response[string]{Body: "ok"}
}

func (c *e2eController) User() httpx.Response[map[string]any] {
	return httpx.Response[map[string]any]{
		Body: map[string]any{"id": float64(43), "version": "0.4.3"},
	}
}

func (c *e2eController) Echo(ctx context.Context, payload []byte) error {
	return pkgws.Send(ctx, pkgws.TextMessage, payload)
}

// TestAppE2EHelperProcess is executed only by TestAppE2E_HTTPWebSocketAndShutdown's
// child process. Keeping the server in a separate process exercises App.Run's
// listener and signal-driven graceful shutdown without signaling the test runner.
func TestAppE2EHelperProcess(t *testing.T) {
	if os.Getenv(e2eHelperEnv) != "1" {
		return
	}

	address := os.Getenv(e2eAddressEnv)
	if address == "" {
		t.Fatal("E2E helper address is empty")
	}

	app := spine.New()
	app.Constructor(func() *e2eController { return &e2eController{} })
	app.Route(http.MethodGet, "/health", (*e2eController).Health)
	app.Route(http.MethodGet, "/users/43", (*e2eController).User)
	if err := app.WebSocket().Register("/ws/echo", (*e2eController).Echo); err != nil {
		t.Fatalf("register websocket route: %v", err)
	}
	if os.Getenv(e2eSignalOnMountEnv) == "1" {
		app.Transport(func(any) {
			process, err := os.FindProcess(os.Getpid())
			if err == nil {
				_ = process.Signal(os.Interrupt)
			}
		})
	}

	if err := app.Run(boot.Options{
		Address:                address,
		EnableGracefulShutdown: true,
		ShutdownTimeout:        3 * time.Second,
		HTTP:                   &boot.HTTPOptions{},
	}); err != nil {
		t.Fatalf("run E2E helper app: %v", err)
	}
}

func TestAppE2E_HTTPWebSocketAndShutdown(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("graceful signal verification requires Unix process signals")
	}

	address := reserveTCPAddress(t)
	var processOutput bytes.Buffer
	cmd := exec.Command(os.Args[0], "-test.run=^TestAppE2EHelperProcess$")
	cmd.Env = append(os.Environ(), e2eHelperEnv+"=1", e2eAddressEnv+"="+address)
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

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(baseURL + "/users/43")
	if err != nil {
		t.Fatalf("GET /users/43: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /users/43 status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode /users/43 response: %v", err)
	}
	if body["id"] != float64(43) || body["version"] != "0.4.3" {
		t.Fatalf("unexpected /users/43 response: %#v", body)
	}

	wsURL := "ws://" + address + "/ws/echo"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	payload := []byte(`{"message":"real-network-e2e"}`)
	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		_ = conn.Close()
		t.Fatalf("write websocket message: %v", err)
	}
	messageType, echoed, err := conn.ReadMessage()
	_ = conn.Close()
	if err != nil {
		t.Fatalf("read websocket message: %v", err)
	}
	if messageType != websocket.TextMessage || string(echoed) != string(payload) {
		t.Fatalf("unexpected websocket echo: type=%d payload=%q", messageType, echoed)
	}

	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("signal E2E helper process: %v", err)
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

func TestAppE2E_ShutdownSignalDuringStartup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("graceful signal verification requires Unix process signals")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	address := reserveTCPAddress(t)
	var processOutput bytes.Buffer
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAppE2EHelperProcess$")
	cmd.Env = append(
		os.Environ(),
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

	client := &http.Client{Timeout: 250 * time.Millisecond}
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
