package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NARUBROWN/spine"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/gorilla/websocket"
)

func TestReferenceContextsOverHTTPAndWebSocket(t *testing.T) {
	prepared, err := newReferenceApp().Prepare(t.Context(), boot.Options{HTTP: &boot.HTTPOptions{}})
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
	for _, test := range []struct {
		path   string
		status int
	}{
		{"/users/42", http.StatusNotFound},
		{"/headers", http.StatusOK},
	} {
		response, err := server.Client().Get(server.URL + test.path)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil || response.StatusCode != test.status {
			t.Fatalf("%s: status=%d body=%s err=%v", test.path, response.StatusCode, body, readErr)
		}
	}
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws/chat", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.WriteJSON(map[string]string{"message": "hello"}); err != nil {
		t.Fatal(err)
	}
	_, body, err := conn.ReadMessage()
	if err != nil || string(body) != "hello" {
		t.Fatalf("reply=%q err=%v", body, err)
	}
}
