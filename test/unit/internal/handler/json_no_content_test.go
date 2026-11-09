package handler

import (
	"errors"
	"net/http"
	"testing"

	"github.com/NARUBROWN/spine/pkg/httpx"
)

type failingStatusWriter struct {
	*fakeResponseWriter
	err error
}

func (w *failingStatusWriter) WriteStatus(int) error { return w.err }

func TestJSONNoContentSkipsSerializationAndPreservesHeaders(t *testing.T) {
	ctx := newFakeExecutionContext()
	writer := newFakeWriter()
	ctx.SetResponseWriter(writer)
	response := httpx.Response[any]{
		Body: make(chan int),
		Options: httpx.ResponseOptions{
			Status:  http.StatusNoContent,
			Headers: map[string]string{"X-Test": "preserved"},
			Cookies: []httpx.Cookie{{Name: "token", Value: "abc"}},
		},
	}
	if err := (&JSONReturnHandler{}).Handle(response, ctx); err != nil {
		t.Fatal(err)
	}
	if writer.status != http.StatusNoContent || writer.writeJSONCalls != 0 || writer.headers["X-Test"] != "preserved" || len(writer.setCookies) != 1 || writer.setCookies[0] != "token=abc" {
		t.Fatalf("unexpected status, body or headers: %+v", writer)
	}
}

func TestJSONNoContentPropagatesStatusWriteError(t *testing.T) {
	ctx := newFakeExecutionContext()
	want := errors.New("status write failed")
	ctx.SetResponseWriter(&failingStatusWriter{fakeResponseWriter: newFakeWriter(), err: want})
	if err := (&JSONReturnHandler{}).Handle(httpx.Response[any]{Options: httpx.ResponseOptions{Status: http.StatusNoContent}}, ctx); !errors.Is(err, want) {
		t.Fatalf("status write error: %v", err)
	}
}
