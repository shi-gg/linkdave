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
	defaultPort     = ":8080"
	version         = "1.0.0"
	drainTimeoutSec = 30 // Time to wait for players to migrate before force shutdown
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	logger.Info("starting linkdave",
		slog.String("version", version),
	)

	port := os.Getenv("LINKDAVE_PORT")
	if port == "" {
		port = defaultPort
	}

	voiceManager := voice.NewManager(logger)
	wsServer := server.NewServer(logger, voiceManager)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", wsServer.HandleWebSocket)
	mux.Handle("/health", server.NewHealthHandler(wsServer, version))
	mux.Handle("/stats", server.NewStatsHandler(wsServer))
	wsServer.RegisterRoutes(mux)

	httpServer := &http.Server{
		Addr:         port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	errChan := make(chan error, 1)

	go func() {
		logger.Info("server listening", slog.String("addr", port))
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

	drainDeadline := drainTimeoutSec * 1000
	wsServer.Drain("shutdown", int64(drainDeadline))

	drainCtx, drainCancel := context.WithTimeout(context.Background(), time.Duration(drainTimeoutSec)*time.Second)
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
			playerCount := wsServer.PlayerCount()
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
