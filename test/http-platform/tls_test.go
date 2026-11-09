package httpplatform_test

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NARUBROWN/spine"
	httpcontract "github.com/NARUBROWN/spine/contract/http"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/route"
)

func tlsMaterial(t *testing.T) (*tls.Config, *tls.Config) {
	t.Helper()
	seed := httptest.NewTLSServer(http.NotFoundHandler())
	defer seed.Close()
	certificate := seed.TLS.Certificates[0]
	certificate.Certificate = append([][]byte(nil), certificate.Certificate...)
	for i := range certificate.Certificate {
		certificate.Certificate[i] = append([]byte(nil), certificate.Certificate[i]...)
	}
	certificate.Leaf = nil
	server := &tls.Config{MinVersion: tls.VersionTLS12, NextProtos: []string{"http/1.1"}, Certificates: []tls.Certificate{certificate}}
	client := seed.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	client.NextProtos = []string{"http/1.1"}
	return server, client
}

func tlsServer(t *testing.T, p spine.PreparedApplication, trust *tls.Config, options ...httpcontract.ServeOption) (string, *http.Client) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- p.HTTPAdapter().Serve(listener, options...) }()
	transport := &http.Transport{TLSClientConfig: trust.Clone()}
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	t.Cleanup(func() {
		transport.CloseIdleConnections()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := p.Close(ctx); err != nil {
			t.Error(err)
		}
		select {
		case err := <-done:
			if !errors.Is(err, httpcontract.ErrServerClosed) {
				t.Errorf("TLS Serve 종료: %v", err)
			}
		case <-ctx.Done():
			t.Error("TLS Serve 종료 시간 초과")
		}
	})
	return "https://" + listener.Addr().String(), client
}

func TestHTTPSPreparedDefaultsAndServeOptions(t *testing.T) {
	for _, engine := range []string{"echo", "nethttp"} {
		for _, mode := range []string{"prepared", "explicit", "override", "snapshot", "provider", "client-config-provider"} {
			t.Run(engine+"/"+mode, func(t *testing.T) {
				serverConfig, trust := tlsMaterial(t)
				if mode == "prepared" {
					// 인증서만 지정한 일반 앱 구성도 별도 ALPN 설정 없이 실행합니다.
					serverConfig.NextProtos = nil
				}
				var providerCalls atomic.Int32
				app := spine.New(spine.WithHTTPAdapter(platformFactory(engine, nativeHooks{})))
				c := &inputController{}
				app.Constructor(func() *inputController { return c })
				app.Route("POST", "/body", (*inputController).Body)
				options := boot.HTTPOptions{TLS: serverConfig}
				var serve []httpcontract.ServeOption
				if mode == "explicit" {
					options.TLS = nil
					serve = []httpcontract.ServeOption{httpcontract.WithTLS(serverConfig)}
				}
				if mode == "override" {
					options.TLS = (httpcontract.ServeOptions{TLS: serverConfig}).Clone().TLS
					options.TLS.MaxVersion = tls.VersionTLS12
					serverConfig.MinVersion = tls.VersionTLS13
					trust.MinVersion = tls.VersionTLS13
					serve = []httpcontract.ServeOption{httpcontract.WithTLS(serverConfig)}
				}
				if mode == "provider" {
					certificate := serverConfig.Certificates[0]
					serverConfig.Certificates = nil
					serverConfig.GetCertificate = func(*tls.ClientHelloInfo) (*tls.Certificate, error) { providerCalls.Add(1); return &certificate, nil }
				}
				if mode == "client-config-provider" {
					provided := (httpcontract.ServeOptions{TLS: serverConfig}).Clone().TLS
					serverConfig.Certificates = nil
					serverConfig.GetConfigForClient = func(*tls.ClientHelloInfo) (*tls.Config, error) {
						providerCalls.Add(1)
						return provided, nil
					}
				}
				p := prepare(t, app, options)
				if providerCalls.Load() != 0 {
					t.Fatal("Prepare에서 인증서 제공자를 실행했습니다")
				}
				if mode == "snapshot" {
					serverConfig.NextProtos[0] = "changed"
					serverConfig.Certificates[0].Certificate[0][0] = 0
					serverConfig.MinVersion = 1
				}
				url, client := tlsServer(t, p, trust, serve...)
				response, err := client.Post(url+"/body", "application/json", strings.NewReader(`{"name":"tls","count":7}`))
				if err != nil {
					t.Fatal(err)
				}
				data, err := io.ReadAll(response.Body)
				_ = response.Body.Close()
				if err != nil || response.StatusCode != 200 || response.TLS == nil || len(response.TLS.VerifiedChains) == 0 || response.ProtoMajor != 1 || canonical(data) != `{"count":7,"maybe":null,"name":"tls"}` {
					t.Fatalf("HTTPS DTO: status=%d body=%s read=%v", response.StatusCode, data, err)
				}
				if mode == "override" && response.TLS.Version != tls.VersionTLS13 {
					t.Fatal("명시한 TLS 설정이 준비 기본값보다 우선해야 합니다")
				}
				if (mode == "provider" || mode == "client-config-provider") && providerCalls.Load() == 0 {
					t.Fatal("TLS 핸드셰이크에서 인증서 제공자를 호출하지 않았습니다")
				}
			})
		}
	}
}

func TestHTTPSRejectsUntrustedServerCertificateAndPlaintextBeforeController(t *testing.T) {
	for _, engine := range []string{"echo", "nethttp"} {
		t.Run(engine, func(t *testing.T) {
			config, trust := tlsMaterial(t)
			c := &inputController{}
			app := spine.New(spine.WithHTTPAdapter(platformFactory(engine, nativeHooks{})))
			app.Constructor(func() *inputController { return c })
			app.Route("POST", "/body", (*inputController).Body)
			url, _ := tlsServer(t, prepare(t, app, boot.HTTPOptions{TLS: config}), trust)
			transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: x509.NewCertPool()}}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
			if response, err := client.Post(url+"/body", "application/json", strings.NewReader(`{}`)); err == nil {
				_ = response.Body.Close()
				t.Fatal("신뢰하지 않는 인증서를 수용했습니다")
			}
			conn, err := net.DialTimeout("tcp", strings.TrimPrefix(url, "https://"), time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
			_, _ = io.WriteString(conn, "POST /body HTTP/1.1\r\nHost: localhost\r\nContent-Length: 2\r\n\r\n{}")
			response, err := http.ReadResponse(bufio.NewReader(conn), nil)
			if err == nil {
				_ = response.Body.Close()
				if response.StatusCode < 400 {
					t.Fatalf("TLS 포트의 평문 요청을 처리했습니다: %d", response.StatusCode)
				}
			}
			if c.calls.Load() != 0 {
				t.Fatal("TLS 검증 전에 컨트롤러가 실행되었습니다")
			}
		})
	}
}

func TestHTTPSAuthenticationAndBodyLimit(t *testing.T) {
	for _, engine := range []string{"echo", "nethttp"} {
		for _, reject := range []bool{true, false} {
			t.Run(engine+"/"+map[bool]string{true: "auth", false: "limit"}[reject], func(t *testing.T) {
				config, trust := tlsMaterial(t)
				var reads atomic.Int32
				done := make(chan struct{})
				c := &uploadController{t: t}
				app := spine.New(spine.WithHTTPAdapter(platformFactory(engine, nativeHooks{before: func(r *http.Request) { r.Body = countingBody{r.Body, &reads} }, after: func(*http.Request) { close(done) }})))
				app.Constructor(func() *uploadController { return c })
				interceptor := &uploadInterceptor{controller: c, reject: reject}
				app.Route("POST", "/upload", (*uploadController).FormFirst, route.WithInterceptors(interceptor))
				url, client := tlsServer(t, prepare(t, app, boot.HTTPOptions{TLS: config, MaxBodyBytes: 32}), trust)
				body, media := multipartPayload(t)
				r, _ := http.NewRequest("POST", url+"/upload", strings.NewReader(string(body)))
				r.ContentLength = -1
				r.Header.Set("Content-Type", media)
				response, err := client.Do(r)
				if err != nil {
					t.Fatal(err)
				}
				data, err := io.ReadAll(response.Body)
				_ = response.Body.Close()
				waitSignal(t, done)
				want := 413
				if reject {
					want = 401
				}
				if err != nil || response.StatusCode != want || strings.Contains(strings.Join(c.order, ","), "controller") {
					t.Fatalf("TLS 인증/한도: %d %s %v", response.StatusCode, data, err)
				}
				if reject && reads.Load() != 0 {
					t.Fatalf("인증 전 읽기: %d", reads.Load())
				}
				if !reject {
					var failure struct {
						In string `json:"in"`
					}
					_ = json.Unmarshal(data, &failure)
					if failure.In != "body" {
						t.Fatalf("입력 출처: %s", data)
					}
				}
			})
		}
	}
}

func TestHTTPSMultipartLifetimeAndNativeDrain(t *testing.T) {
	for _, engine := range []string{"echo", "nethttp"} {
		t.Run(engine, func(t *testing.T) {
			config, trust := tlsMaterial(t)
			entered, release := make(chan context.Context, 1), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			c := &uploadController{t: t}
			app := spine.New(spine.WithHTTPAdapter(platformFactory(engine, nativeHooks{after: func(r *http.Request) {
				if _, err := os.Stat(c.path); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("TLS Execute 뒤 파일 정리: %v", err)
				}
				entered <- r.Context()
				<-release
			}})))
			app.Constructor(func() *uploadController { return c })
			app.Route("POST", "/upload", (*uploadController).FilesFirst, route.WithInterceptors(&uploadInterceptor{controller: c}))
			p := prepare(t, app, boot.HTTPOptions{TLS: config, MultipartMemoryBytes: 1})
			url, client := tlsServer(t, p, trust)
			t.Cleanup(unblock)
			done := make(chan error, 1)
			body, media := multipartPayload(t)
			go func() {
				response, err := client.Post(url+"/upload", media, strings.NewReader(string(body)))
				if err == nil {
					_, err = io.Copy(io.Discard, response.Body)
					_ = response.Body.Close()
					if response.StatusCode != 200 {
						err = errors.New("upload status")
					}
				}
				done <- err
			}()
			var active context.Context
			select {
			case active = <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("TLS 후처리 진입 시간 초과")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			if err := p.Close(ctx); !errors.Is(err, context.DeadlineExceeded) || active.Err() != nil {
				t.Fatalf("TLS 후처리 보존: %v %v", err, active.Err())
			}
			unblock()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("TLS 요청 완료 시간 초과")
			}
			ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel2()
			if err := p.Close(ctx2); err != nil || active.Err() != context.Canceled {
				t.Fatalf("TLS 재대기/취소: %v %v", err, active.Err())
			}
		})
	}
}

type logWriter func([]byte) (int, error)

type tlsCancelController struct {
	entered  chan struct{}
	canceled chan struct{}
}

func (c *tlsCancelController) Wait(ctx context.Context) {
	close(c.entered)
	<-ctx.Done()
	close(c.canceled)
}

func TestTLSClientDisconnectCancelsRequest(t *testing.T) {
	for _, engine := range []string{"echo", "nethttp"} {
		t.Run(engine, func(t *testing.T) {
			config, trust := tlsMaterial(t)
			c := &tlsCancelController{entered: make(chan struct{}), canceled: make(chan struct{})}
			app := spine.New(spine.WithHTTPAdapter(platformFactory(engine, nativeHooks{})))
			app.Constructor(func() *tlsCancelController { return c })
			app.Route("GET", "/wait", (*tlsCancelController).Wait)
			url, _ := tlsServer(t, prepare(t, app, boot.HTTPOptions{TLS: config}), trust)
			conn, err := tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", strings.TrimPrefix(url, "https://"), trust)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = conn.Close() })
			if _, err := io.WriteString(conn, "GET /wait HTTP/1.1\r\nHost: localhost\r\n\r\n"); err != nil {
				t.Fatal(err)
			}
			waitSignal(t, c.entered)
			_ = conn.Close()
			waitSignal(t, c.canceled)
		})
	}
}

func (f logWriter) Write(p []byte) (int, error) { return f(p) }

func TestRunContextStartsHTTPSAndReportsHTTPSOpenAPIURL(t *testing.T) {
	for _, engine := range []string{"echo", "nethttp"} {
		t.Run(engine, func(t *testing.T) {
			config, trust := tlsMaterial(t)
			app := spine.New(spine.WithHTTPAdapter(platformFactory(engine, nativeHooks{})))
			app.Constructor(func() *inputController { return &inputController{} })
			app.Route("POST", "/body", (*inputController).Body)
			urls := make(chan string, 1)
			original := log.Writer()
			log.SetOutput(logWriter(func(p []byte) (int, error) {
				text := string(p)
				if i := strings.Index(text, "[OpenAPI] "); i >= 0 {
					value := strings.TrimSpace(text[i+len("[OpenAPI] "):])
					select {
					case urls <- value:
					default:
					}
				}
				return len(p), nil
			}))
			t.Cleanup(func() { log.SetOutput(original) })
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() {
				done <- app.RunContext(ctx, boot.Options{Address: "127.0.0.1:0", EnableGracefulShutdown: true, HTTP: &boot.HTTPOptions{TLS: config, OpenAPI: boot.OpenAPIOptions{Enabled: true}}})
			}()
			t.Cleanup(func() {
				cancel()
				select {
				case err := <-done:
					if err != nil {
						t.Error(err)
					}
				case <-time.After(5 * time.Second):
					t.Error("TLS RunContext 종료 시간 초과")
				}
			})
			var url string
			select {
			case url = <-urls:
			case err := <-done:
				t.Fatalf("TLS 기동 실패: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("HTTPS URL 출력 시간 초과")
			}
			if !strings.HasPrefix(url, "https://") {
				t.Fatal(url)
			}
			transport := &http.Transport{TLSClientConfig: trust}
			defer transport.CloseIdleConnections()
			response, err := (&http.Client{Transport: transport, Timeout: 5 * time.Second}).Get(url)
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if err != nil || response.StatusCode != 200 || response.TLS == nil || !json.Valid(data) {
				t.Fatalf("실제 관리 HTTPS/OpenAPI: %d %v", response.StatusCode, err)
			}
		})
	}
}
