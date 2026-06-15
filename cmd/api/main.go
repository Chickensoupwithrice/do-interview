package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	apihttp "github.com/example/url-shortener/internal/http"
	"github.com/example/url-shortener/internal/shortener"
	"github.com/example/url-shortener/internal/store"
)

func main() {
	addr := envOrDefault("ADDR", ":8080")
	baseURL := envOrDefault("BASE_URL", "http://localhost:8080")
	dbPath := envOrDefault("DATABASE_PATH", "data/shortener.db")

	sqliteStore, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer sqliteStore.Close()

	service, err := shortener.NewService(sqliteStore, shortener.NewCache(), baseURL)
	if err != nil {
		log.Fatalf("create service: %v", err)
	}
	handler := apihttp.NewHandler(service)

	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()

	log.Printf("listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("serve: %v", err)
	}
	<-shutdownDone
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
