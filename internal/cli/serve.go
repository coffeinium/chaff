package cli

import (
	"context"
	"os/signal"
	"syscall"
	"time"

	"github.com/coffeinium/chaff/internal/api"
	"github.com/coffeinium/chaff/internal/bus"
	"github.com/coffeinium/chaff/internal/config"
	"github.com/coffeinium/chaff/internal/ipc"
	"github.com/coffeinium/chaff/internal/kernel"
	"github.com/coffeinium/chaff/internal/log"
	"github.com/coffeinium/chaff/internal/store"
)

func cmdServe(_ []string) int {
	cfg := config.Load()
	logger := log.New(cfg.LogLevel)

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		logger.Error("открыть стор", "err", err)
		return 1
	}
	defer st.Close()

	k := kernel.New(cfg, logger, st, bus.New())

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := k.Boot(ctx); err != nil {
		logger.Error("boot не удался", "err", err)
		return 1
	}

	srv := ipc.NewServer(cfg.SocketPath, logger)
	registerHandlers(srv, k)
	if err := srv.Listen(); err != nil {
		logger.Error("listen не удался", "socket", cfg.SocketPath, "err", err)
		shutdown(k)
		return 1
	}
	go srv.Serve(ctx)
	logger.Info("chaff готов", "socket", cfg.SocketPath, "db", cfg.DBPath)

	<-ctx.Done()
	logger.Info("останавливаюсь")
	srv.Close()
	shutdown(k)
	return 0
}

func shutdown(k *kernel.Kernel) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	k.Shutdown(ctx)
}

func registerHandlers(srv *ipc.Server, k *kernel.Kernel) {
	for cmd, h := range api.Handlers(k) {
		srv.Handle(cmd, h)
	}
	for cmd, h := range api.TokenHandlers(k) {
		srv.Handle(cmd, h)
	}
}
