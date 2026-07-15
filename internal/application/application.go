package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zxchelik/ozon_bank_test_task/internal/config"
	"github.com/zxchelik/ozon_bank_test_task/internal/handler"
	"github.com/zxchelik/ozon_bank_test_task/internal/metrics"
	"github.com/zxchelik/ozon_bank_test_task/internal/service"
	"github.com/zxchelik/ozon_bank_test_task/internal/storage"
	"github.com/zxchelik/ozon_bank_test_task/internal/storage/memory"
	postgresstorage "github.com/zxchelik/ozon_bank_test_task/internal/storage/postgres"
)

type App struct {
	server          *http.Server
	readiness       *readiness
	shutdownTimeout time.Duration
	closeResources  func()
	logger          *slog.Logger
}

func New(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*App, error) {
	var (
		linkStorage    storage.LinkStorage
		ping           func(context.Context) error
		closeResources = func() {}
	)

	switch cfg.StorageType {
	case config.StorageMemory:
		linkStorage = memory.New()
	case config.StoragePostgres:
		pool, err := pgxpool.New(ctx, cfg.PostgresDSN)
		if err != nil {
			return nil, fmt.Errorf("create postgres pool: %w", err)
		}
		pingCtx, cancel := context.WithTimeout(ctx, cfg.ReadinessTimeout)
		err = pool.Ping(pingCtx)
		cancel()
		if err != nil {
			pool.Close()
			return nil, fmt.Errorf("ping postgres: %w", err)
		}
		linkStorage = postgresstorage.New(pool)
		ping = pool.Ping
		closeResources = pool.Close
	default:
		return nil, fmt.Errorf("unsupported storage type %q", cfg.StorageType)
	}

	ready := &readiness{ping: ping}
	linkService := service.New(linkStorage, service.NewRandomGenerator(), service.DefaultCreateAttempts)
	metricSet := metrics.New(cfg.StorageType)
	router := handler.NewRouter(linkService, ready, metricSet, logger, cfg.BaseURL, cfg.StorageType, cfg.ReadinessTimeout)
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return &App{server: server, readiness: ready, shutdownTimeout: cfg.ShutdownTimeout, closeResources: closeResources, logger: logger}, nil
}

func (a *App) Run(ctx context.Context) error {
	defer a.closeResources()
	errCh := make(chan error, 1)
	a.readiness.set(true)
	go func() { errCh <- a.server.ListenAndServe() }()
	a.logger.Info("server started", "address", a.server.Addr)

	select {
	case err := <-errCh:
		a.readiness.set(false)
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
		a.readiness.set(false)
		a.logger.Info("shutdown started")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), a.shutdownTimeout)
		defer cancel()
		if err := a.server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown HTTP server: %w", err)
		}
		if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP during shutdown: %w", err)
		}
		a.logger.Info("shutdown completed")
		return nil
	}
}
