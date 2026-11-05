package contractprobe

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	contract "github.com/NARUBROWN/spine/contract/http/v3"
)

type probeAdapter interface {
	contract.HTTPAdapter
	external(net.Listener) (func(context.Context) error, <-chan error)
}

type engine struct {
	name    string
	factory func(hooks) contract.Factory
}

var engines = []engine{
	{"nethttp", func(h hooks) contract.Factory { return netFactory{hooks: h} }},
	{"fasthttp", func(h hooks) contract.Factory { return fastFactory{hooks: h} }},
	{"fasthttp-buffered", func(h hooks) contract.Factory { return fastFactory{hooks: h, buffered: true} }},
}

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for probe event")
		var zero T
		return zero
	}
}

func newAdapter(t *testing.T, e engine, execute contract.ExecuteFunc, h hooks, limit int64) probeAdapter {
	t.Helper()
	f := e.factory(h)
	if err := contract.ValidateFactory(f, contract.Requirements{}); err != nil {
		t.Fatal(err)
	}
	a, err := f.New(contract.Config{Execute: execute, Options: contract.ServerOptions{MaxBodyBytes: limit}})
	if err != nil {
		t.Fatal(err)
	}
	if err := contract.ValidateAdapter(a, f.Capabilities()); err != nil {
		t.Fatal(err)
	}
	return a.(probeAdapter)
}

type running struct {
	adapter probeAdapter
	url     string
	client  *http.Client
	address string
}

func start(t *testing.T, adapter probeAdapter, external bool) running {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var done <-chan error
	var stopExternal func(context.Context) error
	if external {
		stopExternal, done = adapter.external(listener)
	} else {
		result := make(chan error, 1)
		go func() { result <- adapter.Serve(listener) }()
		done = result
	}
	transport := &http.Transport{DisableCompression: true}
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	t.Cleanup(func() {
		transport.CloseIdleConnections()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := adapter.Shutdown(ctx); err != nil {
			t.Errorf("adapter cleanup: %v", err)
		}
		if stopExternal != nil {
			if err := stopExternal(ctx); err != nil {
				t.Errorf("external server cleanup: %v", err)
			}
		}
		select {
		case err := <-done:
			if external {
				if err != nil && !errors.Is(err, http.ErrServerClosed) {
					t.Errorf("external Serve: %v", err)
				}
			} else if !errors.Is(err, contract.ErrServerClosed) {
				t.Errorf("managed Serve: %v", err)
			}
		case <-ctx.Done():
			t.Error("server goroutine did not stop")
		}
	})
	return running{adapter: adapter, url: "http://" + listener.Addr().String(), client: client, address: listener.Addr().String()}
}

func perform(t *testing.T, run running, request *http.Request) (int, http.Header, string) {
	t.Helper()
	response, err := run.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, response.Header, string(body)
}

func request(t *testing.T, method, url string, body io.Reader) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func TestRealRequestSnapshotsAndResponseOwnership(t *testing.T) {
	for _, e := range engines {
		t.Run(e.name, func(t *testing.T) {
			type snapshot struct {
				path, query, host, requestID string
				headers                      map[string][]string
				ctx                          context.Context
			}
			snapshots := make(chan snapshot, 32)
			var reads atomic.Int64
			a := newAdapter(t, e, func(s contract.HTTPContext) error {
				before := reads.Load()
				_ = s.Body()
				_ = s.Body()
				if reads.Load() != before {
					return s.Writer().WriteString(500, "getter read the body")
				}
				first := s.Headers()
				first["X-Multi"][0] = "changed"
				if s.Header("x-multi") != "first" {
					return s.Writer().WriteString(500, "headers share mutable memory")
				}
				snapshots <- snapshot{s.RawPath(), s.RawQuery(), s.Header("hOsT"), s.Header("X-Request-ID"), s.Headers(), s.Context()}
				s.Writer().AddHeader("Set-Cookie", "one=1")
				s.Writer().AddHeader("Set-Cookie", "two=2")
				body := json.RawMessage(`{"ok":true}`)
				err := s.Writer().WriteJSON(203, body)
				for i := range body {
					body[i] = 'x'
				}
				return err
			}, hooks{reads: &reads}, -1)
			run := start(t, a, false)
			query := "tags=a,b&tags=c%2Cd&x=+&x=%2B&empty="
			var saved []snapshot
			for i, path := range []string{"/a//b", "/a/../b", "/a/%2F", "/a/%252F", "/later"} {
				req := request(t, "GET", run.url+path+"?"+query, nil)
				req.Host = "Example.COM:8443"
				req.Header["X-Multi"] = []string{"first", "second"}
				req.Header.Set("X-Request-ID", fmt.Sprintf("request-%d", i))
				status, headers, body := perform(t, run, req)
				if status != 203 || body != `{"ok":true}` || headers.Get("Content-Type") != "application/json" || len(headers.Values("Set-Cookie")) != 2 {
					t.Fatalf("response = %d, %v, %q", status, headers, body)
				}
				snap := receive(t, snapshots)
				if snap.path != path || snap.query != query || snap.host != "Example.COM:8443" || strings.Join(snap.headers["X-Multi"], ",") != "first,second" {
					t.Fatalf("snapshot = %+v", snap)
				}
				saved = append(saved, snap)
			}
			for i, snap := range saved {
				select {
				case <-snap.ctx.Done():
				case <-time.After(time.Second):
					t.Fatal("request context was not canceled at return")
				}
				if snap.requestID != fmt.Sprintf("request-%d", i) || snap.ctx.Value(requestIDKey{}) != snap.requestID || snap.headers["X-Multi"][0] != "first" {
					t.Fatalf("snapshot changed after native request reuse: %+v", snap)
				}
			}
		})
	}
}

func TestBodyLimitsOnFixedAndChunkedRequests(t *testing.T) {
	for _, e := range engines {
		t.Run(e.name, func(t *testing.T) {
			for _, chunked := range []bool{false, true} {
				for _, size := range []int{64, 65} {
					t.Run(fmt.Sprintf("chunked=%v/bytes=%d", chunked, size), func(t *testing.T) {
						errorsSeen := make(chan error, 1)
						a := newAdapter(t, e, func(s contract.HTTPContext) error {
							data, err := io.ReadAll(s.Body())
							errorsSeen <- err
							if err != nil {
								var input *contract.RequestError
								if errors.As(err, &input) {
									return s.Writer().WriteString(input.Status, "limited")
								}
								return s.Writer().WriteString(500, err.Error())
							}
							return s.Writer().WriteBytes(200, data)
						}, hooks{}, 64)
						run := start(t, a, false)
						var body io.Reader = strings.NewReader(strings.Repeat("a", size))
						if chunked {
							body = io.NopCloser(body)
						}
						status, _, response := perform(t, run, request(t, "POST", run.url+"/body", body))
						err := receive(t, errorsSeen)
						if size == 64 && (status != 200 || len(response) != 64 || err != nil) {
							t.Fatalf("exact limit: %d %q %v", status, response, err)
						}
						if size == 65 && (status != 413 || !errors.Is(err, errBodyLimit)) {
							t.Fatalf("over limit: %d %q %v", status, response, err)
						}
					})
				}
			}
		})
	}
}

func TestNegativeLimitAndRepeatedBodyGetter(t *testing.T) {
	for _, e := range engines {
		t.Run(e.name, func(t *testing.T) {
			a := newAdapter(t, e, func(s contract.HTTPContext) error {
				prefix := make([]byte, 3)
				if _, err := io.ReadFull(s.Body(), prefix); err != nil {
					return err
				}
				rest, err := io.ReadAll(s.Body())
				if err != nil {
					return err
				}
				return s.Writer().WriteBytes(200, append(prefix, rest...))
			}, hooks{}, -1)
			run := start(t, a, false)
			body := strings.Repeat("unlimited", 1<<15)
			status, _, got := perform(t, run, request(t, "POST", run.url, io.NopCloser(strings.NewReader(body))))
			if status != 200 || got != body {
				t.Fatalf("repeated body getter/unlimited response = %d, %d bytes", status, len(got))
			}
		})
	}
}

func TestRejectedMultipartDoesNotReadOrPreparse(t *testing.T) {
	for _, e := range engines {
		t.Run(e.name, func(t *testing.T) {
			var reads atomic.Int64
			a := newAdapter(t, e, func(s contract.HTTPContext) error {
				_ = s.Body()
				return s.Writer().WriteString(401, "rejected")
			}, hooks{reads: &reads}, 64)
			run := start(t, a, false)
			req := request(t, "POST", run.url, strings.NewReader(strings.Repeat("invalid multipart", 4096)))
			req.Header.Set("Content-Type", "multipart/form-data; boundary=missing")
			status, _, _ := perform(t, run, req)
			if status != 401 || reads.Load() != 0 {
				t.Fatalf("rejection = %d, Body.Read calls = %d", status, reads.Load())
			}
		})
	}
}

func TestTruncatedRequestPreservesReadFailure(t *testing.T) {
	for _, e := range engines {
		t.Run(e.name, func(t *testing.T) {
			cases := []struct{ name, headers, body string }{
				{"fixed", "Content-Length: 8", "ab"},
				{"chunk-data", "Transfer-Encoding: chunked", "8\r\nab"},
				{"chunk-terminator", "Transfer-Encoding: chunked", "2\r\nab\r\n"},
				{"trailer-terminator", "Transfer-Encoding: chunked", "2\r\nab\r\n0\r\n"},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					type readResult struct {
						length int
						err    error
					}
					observed := make(chan readResult, 1)
					a := newAdapter(t, e, func(s contract.HTTPContext) error {
						data, err := io.ReadAll(s.Body())
						observed <- readResult{len(data), err}
						return s.Writer().WriteString(500, "read failed")
					}, hooks{}, -1)
					run := start(t, a, false)
					conn, err := net.DialTimeout("tcp", run.address, time.Second)
					if err != nil {
						t.Fatal(err)
					}
					defer conn.Close()
					_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
					_, _ = fmt.Fprintf(conn, "POST / HTTP/1.1\r\nHost: test\r\n%s\r\n\r\n%s", tc.headers, tc.body)
					_ = conn.(*net.TCPConn).CloseWrite()
					// native 수신 단계에서 오류 응답 또는 연결 종료로 거부해도 유효합니다.
					response, transportErr := http.ReadResponse(bufio.NewReader(conn), nil)
					if response != nil {
						_ = response.Body.Close()
					}
					var result readResult
					select {
					case result = <-observed:
					default:
						var networkTimeout net.Error
						if (response != nil && response.StatusCode < 400) || errors.As(transportErr, &networkTimeout) && networkTimeout.Timeout() {
							t.Fatalf("native rejection did not complete: response=%v error=%v", response, transportErr)
						}
						if response == nil && transportErr == nil {
							t.Fatal("neither native rejection nor Execute result")
						}
						t.Logf("native parser rejected before Execute: response=%v error=%v", response, transportErr)
						return
					}
					err = result.err
					var input *contract.RequestError
					if err == nil || errors.As(err, &input) {
						t.Fatalf("truncated request delivered %d body bytes; error = %T %v", result.length, err, err)
					}
					if tc.name == "fixed" && !errors.Is(err, io.ErrUnexpectedEOF) {
						t.Fatalf("fixed-length truncation lost its I/O cause: %v", err)
					}
				})
			}
		})
	}
}

func TestStopAndShutdownTrackWholeNativeHandler(t *testing.T) {
	for _, e := range engines {
		t.Run(e.name, func(t *testing.T) {
			for _, kind := range []string{"execute", "native", "after"} {
				t.Run(kind, func(t *testing.T) {
					entered, release := make(chan struct{}, 1), make(chan struct{})
					var once sync.Once
					defer once.Do(func() { close(release) })
					block := func() { entered <- struct{}{}; <-release }
					h := hooks{}
					path := "/"
					if kind == "native" {
						h.native, path = block, "/native"
					}
					if kind == "after" {
						h.after = block
					}
					contexts := make(chan context.Context, 1)
					a := newAdapter(t, e, func(s contract.HTTPContext) error {
						contexts <- s.Context()
						if kind == "execute" {
							block()
						}
						return s.Writer().WriteString(200, "complete")
					}, h, -1)
					run := start(t, a, true)
					clientDone := make(chan error, 1)
					go func() {
						r, err := run.client.Get(run.url + path)
						if err == nil {
							_, err = io.Copy(io.Discard, r.Body)
							_ = r.Body.Close()
						}
						clientDone <- err
					}()
					_ = receive(t, entered)
					a.StopAccepting()
					a.StopAccepting()
					status, _, _ := perform(t, run, request(t, "GET", run.url+"/later", nil))
					if status != 503 {
						t.Fatalf("new request after stop = %d", status)
					}
					short, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
					err := a.Shutdown(short)
					cancel()
					if !errors.Is(err, context.DeadlineExceeded) {
						t.Fatalf("shutdown while %s active = %v", kind, err)
					}
					if kind != "native" {
						ctx := receive(t, contexts)
						select {
						case <-ctx.Done():
							t.Fatal("graceful shutdown canceled active native request")
						default:
						}
					}
					once.Do(func() { close(release) })
					if err := receive(t, clientDone); err != nil {
						t.Fatal(err)
					}
					long, cancel := context.WithTimeout(context.Background(), time.Second)
					defer cancel()
					if err := a.Shutdown(long); err != nil {
						t.Fatal(err)
					}
					status, _, _ = perform(t, run, request(t, "GET", run.url+"/later", nil))
					if status != 503 {
						t.Fatalf("external handler after shutdown = %d", status)
					}
				})
			}
		})
	}
}

func TestManagedShutdownAndListenerOwnership(t *testing.T) {
	for _, e := range engines {
		t.Run(e.name, func(t *testing.T) {
			a := newAdapter(t, e, func(s contract.HTTPContext) error { return s.Writer().WriteString(200, "ok") }, hooks{}, -1)
			run := start(t, a, false)
			status, _, _ := perform(t, run, request(t, "GET", run.url, nil))
			if status != 200 {
				t.Fatal(status)
			}
			other, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer other.Close()
			_ = other.(*net.TCPListener).SetDeadline(time.Now().Add(time.Second))
			if err := a.Serve(other); !errors.Is(err, contract.ErrInvalidState) {
				t.Fatalf("second Serve = %v", err)
			}
			if _, err := other.Accept(); !errors.Is(err, net.ErrClosed) {
				t.Fatalf("rejected listener was not closed: %v", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			var group sync.WaitGroup
			for range 8 {
				group.Go(func() {
					if err := a.Shutdown(ctx); err != nil {
						t.Errorf("concurrent Shutdown: %v", err)
					}
				})
			}
			group.Wait()
			if conn, err := net.DialTimeout("tcp", run.address, 100*time.Millisecond); err == nil {
				_ = conn.Close()
				t.Fatal("owned listener still accepts after shutdown")
			}
			if err := a.Serve(nil); !errors.Is(err, contract.ErrInvalidState) {
				t.Fatalf("nil listener = %v", err)
			}
			retry, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer retry.Close()
			_ = retry.(*net.TCPListener).SetDeadline(time.Now().Add(time.Second))
			if err := a.Serve(retry); !errors.Is(err, contract.ErrInvalidState) {
				t.Fatalf("Serve after shutdown = %v", err)
			}
			if _, err := retry.Accept(); !errors.Is(err, net.ErrClosed) {
				t.Fatalf("retry listener was not closed: %v", err)
			}
		})
	}
}

func TestClientDisconnectMatchesDeclaredCapability(t *testing.T) {
	for _, e := range engines {
		t.Run(e.name, func(t *testing.T) {
			contexts, release := make(chan context.Context, 1), make(chan struct{})
			var once sync.Once
			defer once.Do(func() { close(release) })
			a := newAdapter(t, e, func(s contract.HTTPContext) error {
				contexts <- s.Context()
				<-release
				return nil
			}, hooks{}, -1)
			run := start(t, a, false)
			conn, err := net.DialTimeout("tcp", run.address, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = fmt.Fprintf(conn, "GET / HTTP/1.1\r\nHost: test\r\n\r\n")
			ctx := receive(t, contexts)
			_ = conn.Close()
			caps := e.factory(hooks{}).Capabilities()
			if caps.ClientDisconnectCancellation {
				select {
				case <-ctx.Done():
				case <-time.After(time.Second):
					t.Fatal("declared disconnect cancellation was not delivered")
				}
			} else {
				if err := contract.ValidateFactory(e.factory(hooks{}), contract.Requirements{ClientDisconnectCancellation: true}); !errors.Is(err, contract.ErrUnsupportedCapability) {
					t.Fatalf("missing disconnect capability was accepted: %v", err)
				}
				select {
				case <-ctx.Done():
					t.Fatal("probe context canceled before native request returned")
				case <-time.After(30 * time.Millisecond):
				}
			}
			once.Do(func() { close(release) })
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
				t.Fatal("context was not canceled at return")
			}
		})
	}
}

func TestNetHTTPUpgradeAndUnsupportedFastHTTPCombination(t *testing.T) {
	if err := contract.ValidateFactory(fastFactory{}, contract.Requirements{NetHTTPUpgrade: true}); !errors.Is(err, contract.ErrUnsupportedCapability) {
		t.Fatalf("fasthttp with existing WS contract = %v", err)
	}
	entered, release := make(chan struct{}, 1), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	factory := netFactory{}
	adapter, err := factory.NewWithNetHTTPUpgrades(contract.Config{Execute: func(contract.HTTPContext) error { panic("upgrade must bypass Execute") }}, []contract.NetHTTPUpgradeRoute{{Path: "/upgrade", Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: probe\r\n\r\n")
		_ = rw.Flush()
		entered <- struct{}{}
		<-release
	})}})
	if err != nil {
		t.Fatal(err)
	}
	run := start(t, adapter.(probeAdapter), true)
	conn, err := net.DialTimeout("tcp", run.address, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(time.Second))
	_, _ = fmt.Fprintf(conn, "GET /upgrade HTTP/1.1\r\nHost: test\r\nConnection: Upgrade\r\nUpgrade: probe\r\n\r\n")
	response, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil || response.StatusCode != 101 {
		t.Fatalf("native upgrade = %v, %v", response, err)
	}
	_ = receive(t, entered)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	err = adapter.Shutdown(ctx)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("upgrade admission was not tracked: %v", err)
	}
	once.Do(func() { close(release) })
}

func TestUnsupportedNativeOptionsFailBeforeServerConstruction(t *testing.T) {
	cfg := contract.Config{Execute: func(contract.HTTPContext) error { panic("must not execute") }, Options: contract.ServerOptions{WriteTimeout: time.Second}}
	if a, err := (fastFactory{}).New(cfg); a != nil || !errors.Is(err, contract.ErrUnsupportedOption) {
		t.Fatalf("fasthttp WriteTimeout mapping = %v, %v", a, err)
	}
	if a, err := (netFactory{}).New(cfg); a == nil || err != nil {
		t.Fatalf("net/http WriteTimeout mapping = %v, %v", a, err)
	}
}

func TestConcurrentRequestIsolation(t *testing.T) {
	for _, e := range engines {
		t.Run(e.name, func(t *testing.T) {
			a := newAdapter(t, e, func(s contract.HTTPContext) error {
				data, err := io.ReadAll(s.Body())
				if err != nil {
					return err
				}
				return s.Writer().WriteBytes(200, append([]byte(s.Header("X-Request-ID")+":"), data...))
			}, hooks{}, -1)
			run := start(t, a, false)
			var group sync.WaitGroup
			for i := range 32 {
				group.Go(func() {
					value := fmt.Sprint(i)
					req, _ := http.NewRequest("POST", run.url, bytes.NewBufferString(value))
					req.Header.Set("X-Request-ID", value)
					res, err := run.client.Do(req)
					if err != nil {
						t.Error(err)
						return
					}
					defer res.Body.Close()
					data, err := io.ReadAll(res.Body)
					if err != nil || string(data) != value+":"+value {
						t.Errorf("request %s: %q, %v", value, data, err)
					}
				})
			}
			group.Wait()
		})
	}
}

func TestChunkedEOFAndKeepAlive(t *testing.T) {
	for _, e := range engines {
		t.Run(e.name, func(t *testing.T) {
			a := newAdapter(t, e, func(s contract.HTTPContext) error {
				body, err := io.ReadAll(s.Body())
				if err != nil {
					return s.Writer().WriteString(500, err.Error())
				}
				extra, err := io.ReadAll(s.Body())
				if err != nil || len(extra) != 0 {
					return s.Writer().WriteString(500, "body did not remain at EOF")
				}
				return s.Writer().WriteBytes(200, body)
			}, hooks{}, -1)
			run := start(t, a, false)
			for i, body := range []string{"", "valid", strings.Repeat("stream", 4096)} {
				reused := make(chan bool, 1)
				req := request(t, "POST", run.url, io.NopCloser(strings.NewReader(body)))
				if i == 1 {
					req.Trailer = http.Header{"X-Probe": []string{"complete"}}
				}
				req = req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{
					GotConn: func(info httptrace.GotConnInfo) { reused <- info.Reused },
				}))
				status, _, actual := perform(t, run, req)
				if status != 200 || actual != body {
					t.Fatalf("valid chunked body %d: status=%d bytes=%d", i, status, len(actual))
				}
				if got := receive(t, reused); i > 0 && !got {
					t.Fatalf("connection was not reused after valid chunked body %d", i-1)
				}
			}
		})
	}
}
