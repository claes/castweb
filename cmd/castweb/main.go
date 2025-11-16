package main

import (
    "context"
    "flag"
    "log/slog"
    nethttp "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    apphttp "github.com/claes/ytplv/internal/http"
)

func main() {
    // Configure structured logging to stderr
    slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{})))

    // Flags
    var root string
    var ytcastDevice string
    var port string
    var statePath string
	flag.StringVar(&root, "root", "", "root directory containing .strm/.nfo hierarchy (required)")
    flag.StringVar(&ytcastDevice, "ytcast", "", "ytcast device id to cast to (optional)")
    flag.StringVar(&statePath, "state", "/var/lib/castweb", "directory for persistent state (state.json)")
	flag.StringVar(&port, "port", "", "port to listen on (required or set PORT env)")
	flag.Parse()
	if root == "" {
		// accept positional arg if provided
		if flag.NArg() > 0 {
			root = flag.Arg(0)
		}
	}
	if ytcastDevice == "" {
		// allow env var fallback
		ytcastDevice = os.Getenv("YTCAST_DEVICE")
	}
	if port == "" {
		port = "8080"
	}
    if root == "" {
        slog.Error("missing root directory", "hint", "pass -root PATH or positional PATH")
        os.Exit(1)
    }
    if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
        slog.Error("invalid root directory", "root", root, "err", err)
        os.Exit(1)
    }

	mux := apphttp.NewServer(root, ytcastDevice, statePath)

	addr := ":" + port

	srv := &nethttp.Server{
		Addr:              addr,
		Handler:           mux,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)

    errCh := make(chan error, 1)
    go func() {
        slog.Info("server listening", "addr", addr)
        if err := srv.ListenAndServe(); err != nil && err != nethttp.ErrServerClosed {
            errCh <- err
        }
    }()

    select {
    case <-done:
        // proceed to shutdown
    case err := <-errCh:
        slog.Error("listen failed", "err", err)
        os.Exit(1)
    }
    slog.Info("shutdown signal received")

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    if err := srv.Shutdown(ctx); err != nil {
        slog.Warn("graceful shutdown failed", "err", err)
        _ = srv.Close()
    }
    slog.Info("server stopped")
}
