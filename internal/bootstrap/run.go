package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	adapter "github.com/NARUBROWN/spine/contract/http"
)

func Run(config Config) error { return RunContext(context.Background(), config) }

func RunContext(ctx context.Context, config Config) (err error) {
	config = snapshotConfig(config)
	if err := Validate(config); err != nil {
		return err
	}
	if isNilHTTPValue(ctx) {
		ctx = context.Background()
	}
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	// 시작 중 native 훅과 custom Init에 도착한 신호도 준비 취소로 전달합니다.
	if requiresShutdownSignal(config) {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(signals)
		go func() {
			select {
			case <-signals:
				cancelRun()
			case <-runCtx.Done():
			}
		}()
	}
	printBanner()
	application, err := prepare(runCtx, config)
	if err != nil {
		// 관리 실행에서 요청한 취소는 정상 종료입니다. 자원 정리 오류는 보존합니다.
		if runCtx.Err() != nil && err == runCtx.Err() {
			return nil
		}
		return err
	}
	defer func() {
		if closeErr := application.closeWithTimeout(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()
	return application.run(runCtx)
}

func (a *Application) run(ctx context.Context) error {
	if ctx.Err() != nil {
		return nil
	}
	var consumerErrors chan error
	if len(a.consumers) > 0 {
		consumerErrors = make(chan error, len(a.consumers))
	}
	for _, consumer := range a.consumers {
		forwardConsumerErrors(consumer.name, consumer.runtime, consumerErrors)
		go consumer.runtime.Start(ctx)
	}
	var transportErrors chan error
	if len(a.transports) > 0 {
		transportErrors = make(chan error, len(a.transports))
	}
	for _, transport := range a.transports {
		go func() {
			if err := transport.Start(); err != nil {
				transportErrors <- err
			}
		}()
	}
	var httpErrors chan error
	if a.server != nil {
		if ctx.Err() != nil {
			return nil
		}
		address := a.config.Address
		if address == "" {
			address = ":http"
		}
		listener, err := net.Listen("tcp", address)
		if err != nil {
			return fmt.Errorf("[Bootstrap] failed to listen on %s: %w", address, err)
		}
		a.listener = &managedHTTPListener{Listener: listener}
		if ctx.Err() != nil {
			return nil
		}
		httpErrors = make(chan error, 1)
		log.Printf("[Bootstrap] Server listening on: %s", listener.Addr())
		go func() {
			err := a.server.Serve(a.listener)
			if err == nil {
				err = fmt.Errorf("%w: Serve returned nil", adapter.ErrInvalidAdapter)
			}
			if !errors.Is(err, http.ErrServerClosed) {
				httpErrors <- err
			}
		}()
	}
	if a.server == nil && len(a.consumers) == 0 && len(a.transports) == 0 {
		return nil
	}
	select {
	case err := <-httpErrors:
		return err
	case err := <-consumerErrors:
		return err
	case err := <-transportErrors:
		return err
	case <-ctx.Done():
		log.Println("[Bootstrap] Shutdown requested. Stopping runtimes...")
		return nil
	}
}
