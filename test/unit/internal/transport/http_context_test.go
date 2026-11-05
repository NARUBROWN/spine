package transport

import (
	"context"
	"errors"
	bodyinput "github.com/NARUBROWN/spine/internal/input"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	adapter "github.com/NARUBROWN/spine/contract/http"
	"github.com/NARUBROWN/spine/pkg/httperr"
)

type stateWriter struct{ adapter.ResponseWriter }

func TestHTTPContextTypedStateIsIndependentOfUserStore(t *testing.T) {
	ctx := NewHTTPContext(testScope{ctx: context.Background()}, 0)
	writer := &stateWriter{}
	ctx.SetResponseWriter(writer)
	params := map[string]string{"id": "route"}
	keys := []string{"id"}
	ctx.SetRoute(params, keys)
	params["id"], keys[0] = "mutated", "mutated"
	ctx.Set("spine.response_writer", "user value")
	ctx.Set("spine.params", "user value")
	ctx.Set("spine.pathKeys", "user value")
	if ctx.ResponseWriter() != writer || ctx.Param("id") != "route" || ctx.PathKeys()[0] != "id" {
		t.Fatal("사용자 저장소와 입력 map/slice가 내부 실행 상태를 덮어쓰면 안 됩니다")
	}
	returnedKeys := ctx.PathKeys()
	returnedKeys[0] = "mutated"
	if ctx.PathKeys()[0] != "id" {
		t.Fatal("외부에서 반환된 키 목록을 변경해도 라우팅 상태를 보존해야 합니다")
	}
}

type testScope struct {
	adapter.HTTPContext
	ctx context.Context
	err error
}

func (s testScope) Context() context.Context {
	if s.ctx == nil {
		return context.Background()
	}
	return s.ctx
}
func (testScope) Writer() adapter.ResponseWriter { return nil }
func (s testScope) RawPath() string              { return "/a%252Fb/../c" }
func (s testScope) RawQuery() string             { return "tags=a%2Cb,c&empty=&bad=%ZZ&good=ok" }
func (s testScope) Header(string) string         { return "application/json" }
func (s testScope) Body() io.Reader {
	if s.err != nil {
		return failingBody{s.err}
	}
	return strings.NewReader(`{}`)
}

type failingBody struct{ err error }

func (r failingBody) Read([]byte) (int, error) { return 0, r.err }

type testEvent struct{}

func (testEvent) Name() string          { return "test" }
func (testEvent) OccurredAt() time.Time { return time.Time{} }

func TestHTTPContextOwnsSpineStatePerRequest(t *testing.T) {
	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "carried")
	first := NewHTTPContext(testScope{ctx: ctx}, 0)
	second := NewHTTPContext(testScope{ctx: ctx}, 0)
	first.Set("value", true)
	if _, exists := second.Get("value"); exists || first.Context().Value(key{}) != "carried" {
		t.Fatal("요청 context를 보존하고 요청 store를 격리해야 합니다")
	}
	first.SetRoute(map[string]string{"id": "spine"}, []string{"id"})
	params := first.Params()
	params["id"] = "mutated"
	if first.Param("id") != "spine" || second.Param("id") != "" || len(first.PathKeys()) != 1 {
		t.Fatal("Spine 라우팅 결과와 native 파라미터를 분리해야 합니다")
	}
	first.EventBus().Publish(testEvent{})
	if len(second.EventBus().Drain()) != 0 || len(first.EventBus().Drain()) != 1 || len(first.EventBus().Drain()) != 0 {
		t.Fatal("이벤트 버스는 요청마다 생성하고 drain 결과를 격리해야 합니다")
	}
}

func TestHTTPContextConvertsContractErrorsAndPreservesCause(t *testing.T) {
	cause := errors.New("invalid input")
	input := &adapter.RequestError{Status: 415, Message: "Unsupported Media Type", Cause: cause}
	ctx := NewHTTPContext(testScope{err: input}, 0)
	for _, err := range []error{ctx.Bind(nil)} {
		var got *httperr.HTTPError
		if !errors.As(err, &got) || got.Status != 415 || got.Message != input.Message || !errors.Is(err, cause) {
			t.Fatalf("입력 오류의 상태와 원인을 보존해야 합니다: %v", err)
		}
	}
	limit := &http.MaxBytesError{Limit: 64}
	if got := NewHTTPContext(testScope{err: limit}, 0).Bind(nil); !errors.Is(got, limit) {
		t.Fatalf("크기 초과 오류는 커널의 413 처리를 위해 보존해야 합니다: %v", got)
	}
}

func TestHTTPContextUsesStatusTextWhenContractMessageIsEmpty(t *testing.T) {
	err := NewHTTPContext(testScope{err: &adapter.RequestError{Status: 400}}, 0).Bind(nil)
	var got *httperr.HTTPError
	if !errors.As(err, &got) || got.Message != "Bad Request" {
		t.Fatalf("계약 오류의 기본 메시지를 보존해야 합니다: %v", err)
	}
}

func TestHTTPContextParsesRawViewsWithoutNativeNormalization(t *testing.T) {
	ctx := NewHTTPContext(testScope{}, 0)
	if ctx.Path() != "/a%2Fb/../c" || ctx.RawQuery() != (testScope{}).RawQuery() || ctx.Query("tags") != "a,b,c" || ctx.Query("good") != "ok" {
		t.Fatal("원시 경로를 한 번만 디코딩하고 기존 쿼리 조회를 보존해야 합니다")
	}
	values := ctx.Queries()
	values["good"][0] = "changed"
	if ctx.Query("good") != "ok" {
		t.Fatal("쿼리 뷰를 요청마다 격리해야 합니다")
	}
	if err := ctx.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if err := ctx.Bind(nil); !errors.Is(err, bodyinput.ErrBodyClosed) {
		t.Fatalf("정리 뒤 바인딩을 거부해야 합니다: %v", err)
	}
}
