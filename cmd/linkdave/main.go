package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/shi-gg/linkdave/server/server"
	"github.com/shi-gg/linkdave/server/voice"
)

const (
	PORT              = ":8080"
	DRAIN_TIMEOUT_SEC = 30 // Time to wait for players to migrate before force shutdown
)

var version = os.Getenv("VERSION")

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	logger.Info("starting linkdave",
		slog.String("version", version),
	)

	voiceManager := voice.NewManager(logger)
	server := server.NewServer(logger, voiceManager)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", server.HandleWebSocket)
	server.RegisterRoutes(mux)

	httpServer := &http.Server{
		Addr:         PORT,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	errChan := make(chan error, 1)

	go func() {
		logger.Info("server listening", slog.String("addr", PORT))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		logger.Info("received shutdown signal", slog.String("signal", sig.String()))
	case err := <-errChan:
		logger.Error("server error", slog.Any("error", err))
	}

	server.Drain("shutdown", int64(DRAIN_TIMEOUT_SEC))

	drainCtx, drainCancel := context.WithTimeout(context.Background(), DRAIN_TIMEOUT_SEC*time.Second)
	defer drainCancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

DrainLoop:
	for {
		select {
		case <-drainCtx.Done():
			logger.Warn("drain timeout reached, forcing shutdown")
			break DrainLoop
		case <-ticker.C:
			playerCount := server.PlayerCount()
			if playerCount == 0 {
				logger.Info("all players migrated successfully")
				break DrainLoop
			}
			logger.Info("waiting for player migration", slog.Int("remaining_players", playerCount))
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	logger.Info("shutting down servers...")

	voiceManager.Close()

	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Error("server shutdown error", slog.Any("error", err))
	}

	logger.Info("linkdave stopped")
}
