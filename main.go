package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/agent-orchestration/ledgerly/pkg/handler"
	"github.com/agent-orchestration/ledgerly/pkg/server"
	"github.com/agent-orchestration/ledgerly/pkg/service"
)

func main() {
	// -----------------------------------------------------------------------
	// Logger setup - RULE-OBS-01: structured JSON logging
	// -----------------------------------------------------------------------
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	logger.InfoContext(context.Background(), "starting ledgerly invoice service",
		slog.String("component", "main"),
	)

	// -----------------------------------------------------------------------
	// Dependency injection - Clean Architecture layers wired together
	// -----------------------------------------------------------------------

	// For now, we start with nil repositories since we don't have a database.
	// In production, these would be initialized with proper connection strings.
	invoiceSvc := service.NewInvoiceService(nil, nil, nil, nil)

	// Create HTTP handler
	invoiceHandler := handler.NewInvoiceHandler(invoiceSvc, logger, nil)

	// Create router
	httpHandler := handler.NewRouter(invoiceHandler, logger)

	// Create HTTP server
	srv := server.NewServer(":8080", httpHandler, logger)

	// -----------------------------------------------------------------------
	// Graceful shutdown - RULE-RES-02: bounded goroutine lifecycle
	// -----------------------------------------------------------------------
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.Start(); err != nil && !server.IsErrServerClosed(err) {
			logger.ErrorContext(context.Background(), "HTTP server error",
				slog.String("component", "main"),
				slog.Any("error", err),
			)
			os.Exit(1)
		}
	}()

	// Block until signal received
	sig := <-quit
	logger.InfoContext(context.Background(), "received shutdown signal",
		slog.String("component", "main"),
		slog.String("signal", sig.String()),
	)

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.ErrorContext(context.Background(), "HTTP server shutdown error",
			slog.String("component", "main"),
			slog.Any("error", err),
		)
		os.Exit(1)
	}

	logger.InfoContext(context.Background(), "server shutdown complete",
		slog.String("component", "main"),
	)
}
