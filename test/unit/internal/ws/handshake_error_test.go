package ws

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NARUBROWN/spine/pkg/httperr"
)

func TestHandshakeRejectionPreservesHTTPErrorStatus(t *testing.T) {
	for _, status := range []int{200, 302, 399, 400, 403, 429, 500, 503, 599, 600} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			writer := httptest.NewRecorder()
			failure := &httperr.HTTPError{Status: status, Message: "private failure detail"}
			writeHandshakeRejection(writer, fmt.Errorf("handshake: %w", failure))
			want := status
			if status < 400 || status > 599 {
				want = http.StatusInternalServerError
			}
			if writer.Code != want || strings.Contains(writer.Body.String(), failure.Message) {
				t.Fatalf("핸드셰이크 오류 상태를 유지하고 내부 메시지는 숨겨야 합니다: status=%d body=%q", writer.Code, writer.Body.String())
			}
		})
	}
}

func TestHandshakeRejectionKeepsGenericAuthenticationFailure(t *testing.T) {
	writer := httptest.NewRecorder()
	writeHandshakeRejection(writer, errors.New("private authentication failure"))
	if writer.Code != http.StatusUnauthorized || writer.Body.String() != "Unauthorized\n" {
		t.Fatalf("일반 인증 거부는 401이어야 합니다: status=%d body=%q", writer.Code, writer.Body.String())
	}
	var failure *httperr.HTTPError
	writer = httptest.NewRecorder()
	writeHandshakeRejection(writer, failure)
	if writer.Code != http.StatusInternalServerError {
		t.Fatalf("유효하지 않은 HTTPError는 500이어야 합니다: %d", writer.Code)
	}
}
