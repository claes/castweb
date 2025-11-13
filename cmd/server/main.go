package main

import (
	"context"
	"flag"
	"log"
	nethttp "net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	apphttp "github.com/claes/ytplv/internal/http"
)

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	// Flags
	var root string
	flag.StringVar(&root, "root", "", "root directory containing .strm/.nfo hierarchy (required)")
	flag.Parse()
	if root == "" {
		// accept positional arg if provided
		if flag.NArg() > 0 {
			root = flag.Arg(0)
		}
	}
	if root == "" {
		log.Fatal("missing root directory: pass -root PATH or positional PATH")
	}
	if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
		log.Fatalf("invalid root directory: %s", root)
	}

	mux := apphttp.NewServer(root)

	port := getenv("PORT", "8080")
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

	go func() {
		log.Printf("server listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != nethttp.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-done
	log.Println("shutdown signal received")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
		_ = srv.Close()
	}
	log.Println("server stopped")
}
