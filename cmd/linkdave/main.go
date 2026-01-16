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
	defaultWSPort   = ":8080"
	defaultHTTPPort = ":8081"
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

	wsPort := os.Getenv("LINKDAVE_WS_PORT")
	if wsPort == "" {
		wsPort = defaultWSPort
	}

	httpPort := os.Getenv("LINKDAVE_HTTP_PORT")
	if httpPort == "" {
		httpPort = defaultHTTPPort
	}

	voiceManager := voice.NewManager(logger)
	wsServer := server.NewServer(logger, voiceManager)

	wsMux := http.NewServeMux()
	wsMux.HandleFunc("/ws", wsServer.HandleWebSocket)

	httpMux := http.NewServeMux()
	httpMux.Handle("/health", server.NewHealthHandler(wsServer, version))
	httpMux.Handle("/stats", server.NewStatsHandler(wsServer))

	wsHTTPServer := &http.Server{
		Addr:         wsPort,
		Handler:      wsMux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	healthHTTPServer := &http.Server{
		Addr:         httpPort,
		Handler:      httpMux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	errChan := make(chan error, 2)

	go func() {
		logger.Info("websocket server listening", slog.String("addr", wsPort))
		if err := wsHTTPServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	go func() {
		logger.Info("health server listening", slog.String("addr", httpPort))
		if err := healthHTTPServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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

	if err := wsHTTPServer.Shutdown(ctx); err != nil {
		logger.Error("websocket server shutdown error", slog.Any("error", err))
	}
	if err := healthHTTPServer.Shutdown(ctx); err != nil {
		logger.Error("health server shutdown error", slog.Any("error", err))
	}

	logger.Info("linkdave stopped")
}
