package contractprobe

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"

	contract "github.com/NARUBROWN/spine/contract/http/v3"
	"github.com/valyala/fasthttp"
)

type fastFactory struct {
	hooks    hooks
	buffered bool
}

func (fastFactory) Capabilities() contract.Capabilities {
	return contract.Capabilities{ContractVersion: contract.ContractVersion, ServerOptions: contract.ServerOptionSupport{ReadTimeout: true, IdleTimeout: true}}
}

func (f fastFactory) New(cfg contract.Config) (contract.HTTPAdapter, error) {
	if err := cfg.Validate(f.Capabilities()); err != nil {
		return nil, err
	}
	a := &fastAdapter{lifecycle: newLifecycle(), cfg: cfg, hooks: f.hooks}
	// 초기 버퍼를 작게 두어 실제 스트림과 읽기 한도를 함께 검증합니다.
	threshold := 1
	if f.buffered {
		// 검증 입력은 작게 제한하며 공개 제품의 메모리/한도 정책으로 채택하지 않습니다.
		threshold = math.MaxInt
	}
	a.server = &fasthttp.Server{Handler: a.handle, StreamRequestBody: !f.buffered, DisablePreParseMultipartForm: true, MaxRequestBodySize: threshold, ReadTimeout: cfg.Options.ReadTimeout, IdleTimeout: cfg.Options.IdleTimeout}
	return a, nil
}

type fastAdapter struct {
	*lifecycle
	cfg    contract.Config
	hooks  hooks
	server *fasthttp.Server
}

func (a *fastAdapter) handle(native *fasthttp.RequestCtx) {
	if !a.enter() {
		native.SetStatusCode(503)
		native.SetBodyString("application is closing")
		return
	}
	defer a.leave()
	if string(native.URI().PathOriginal()) == "/native" && a.hooks.native != nil {
		a.hooks.native()
		native.SetBodyString("native")
		return
	}
	headers := make(http.Header)
	native.Request.Header.VisitAll(func(key, value []byte) { headers.Add(string(key), string(value)) })
	headers.Set("Host", string(native.Request.Header.Host()))
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), requestIDKey{}, headers.Get("X-Request-ID")))
	defer cancel()
	var source io.Reader = native.Request.BodyStream()
	if source == nil {
		source = bytes.NewReader(native.Request.Body())
	}
	writer := &response{headers: make(http.Header), sink: func(status int, headers http.Header, body []byte) error {
		for key, values := range headers {
			native.Response.Header.Del(key)
			for _, value := range values {
				native.Response.Header.Add(key, value)
			}
		}
		native.SetStatusCode(status)
		native.Response.SetBodyRaw(body)
		return nil
	}}
	s := &scope{ctx: ctx, method: string(native.Method()), path: string(native.URI().PathOriginal()), rawQuery: string(native.URI().QueryString()), headers: headers, body: bodyReader(source, a.cfg.Options.MaxBodyBytes, int64(native.Request.Header.ContentLength()), a.hooks.reads), writer: writer}
	_ = a.cfg.Execute(s)
	if a.hooks.after != nil {
		a.hooks.after()
	}
}

func (a *fastAdapter) Serve(listener net.Listener) error {
	if err := a.claim(listener); err != nil {
		return err
	}
	defer listener.Close()
	err := a.server.Serve(listener)
	if err == nil {
		if a.isStopping() {
			return contract.ErrServerClosed
		}
		return fmt.Errorf("%w: unexpected nil Serve result", contract.ErrInvalidAdapter)
	}
	return err
}

func (a *fastAdapter) Shutdown(ctx context.Context) error {
	return a.shutdown(ctx, a.server.Shutdown)
}

func (a *fastAdapter) external(listener net.Listener) (func(context.Context) error, <-chan error) {
	server := &fasthttp.Server{Handler: a.handle, StreamRequestBody: a.server.StreamRequestBody, DisablePreParseMultipartForm: true, MaxRequestBodySize: a.server.MaxRequestBodySize}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	return server.ShutdownWithContext, done
}
