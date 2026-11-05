package contractprobe

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"

	contract "github.com/NARUBROWN/spine/contract/http"
)

type netFactory struct{ hooks hooks }

func (netFactory) Capabilities() contract.Capabilities {
	return contract.Capabilities{
		ContractVersion: contract.ContractVersion, NetHTTPHandler: true, NetHTTPUpgrade: true,
		ClientDisconnectCancellation: true,
		ServerOptions:                contract.ServerOptionSupport{ReadHeaderTimeout: true, ReadTimeout: true, WriteTimeout: true, IdleTimeout: true, MaxHeaderBytes: true},
	}
}

func (f netFactory) New(cfg contract.Config) (contract.HTTPAdapter, error) {
	return f.NewWithNetHTTPUpgrades(cfg, nil)
}

func (f netFactory) NewWithNetHTTPUpgrades(cfg contract.Config, upgrades []contract.NetHTTPUpgradeRoute) (contract.HTTPAdapter, error) {
	if err := cfg.Validate(f.Capabilities()); err != nil {
		return nil, err
	}
	if err := contract.ValidateNetHTTPUpgradeRoutes(upgrades); err != nil {
		return nil, err
	}
	a := &netAdapter{lifecycle: newLifecycle(), cfg: cfg, hooks: f.hooks, upgrades: make(map[string]http.Handler)}
	for _, route := range upgrades {
		a.upgrades[route.Path] = route.Handler
	}
	a.handler = http.HandlerFunc(a.handle)
	o := cfg.Options
	a.server = &http.Server{Handler: a.handler, ReadHeaderTimeout: o.ReadHeaderTimeout, ReadTimeout: o.ReadTimeout, WriteTimeout: o.WriteTimeout, IdleTimeout: o.IdleTimeout, MaxHeaderBytes: o.MaxHeaderBytes}
	return a, nil
}

type netAdapter struct {
	*lifecycle
	cfg      contract.Config
	hooks    hooks
	server   *http.Server
	handler  http.Handler
	upgrades map[string]http.Handler
}

func (a *netAdapter) Handler() http.Handler { return a.handler }

func (a *netAdapter) handle(w http.ResponseWriter, r *http.Request) {
	if !a.enter() {
		http.Error(w, "application is closing", 503)
		return
	}
	defer a.leave()
	if upgrade := a.upgrades[r.URL.Path]; upgrade != nil && r.Method == http.MethodGet {
		upgrade.ServeHTTP(w, r)
		return
	}
	if r.URL.Path == "/native" && a.hooks.native != nil {
		a.hooks.native()
		_, _ = w.Write([]byte("native"))
		return
	}
	headers := r.Header.Clone()
	headers.Set("Host", r.Host)
	ctx, cancel := context.WithCancel(context.WithValue(r.Context(), requestIDKey{}, strings.Clone(headers.Get("X-Request-ID"))))
	defer cancel()
	writer := &response{headers: make(http.Header), sink: func(status int, headers http.Header, body []byte) error {
		for key, values := range headers {
			w.Header()[key] = append([]string(nil), values...)
		}
		w.WriteHeader(status)
		_, err := w.Write(body)
		return err
	}}
	s := &scope{ctx: ctx, method: r.Method, path: r.URL.EscapedPath(), rawQuery: r.URL.RawQuery, headers: headers, body: bodyReader(r.Body, a.cfg.Options.MaxBodyBytes, r.ContentLength, a.hooks.reads), writer: writer}
	_ = a.cfg.Execute(s)
	if a.hooks.after != nil {
		a.hooks.after()
	}
}

func (a *netAdapter) Serve(listener net.Listener) error {
	if err := a.claim(listener); err != nil {
		return err
	}
	defer listener.Close()
	err := a.server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) && a.isStopping() {
		return contract.ErrServerClosed
	}
	return err
}

func (a *netAdapter) Shutdown(ctx context.Context) error {
	return a.shutdown(ctx, func() error { return a.server.Shutdown(context.Background()) })
}

func (a *netAdapter) external(listener net.Listener) (func(context.Context) error, <-chan error) {
	server := &http.Server{Handler: a.handler}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	return server.Shutdown, done
}
