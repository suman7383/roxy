package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/suman7383/roxy/internals"
	"github.com/suman7383/roxy/internals/backend"
	"github.com/suman7383/roxy/internals/config"
	"github.com/suman7383/roxy/internals/health"
)

const (
	defaultConfigPath = "config.yaml"
	startHealthy      = true
)

func main() {
	configPath := defaultConfigPath

	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	config, err := config.Load(configPath)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Config loaded:", config)

	var backends []*backend.Backend
	for _, rawURL := range config.Backends {
		u, err := url.Parse(rawURL)
		if err != nil {
			log.Fatal(err)
		}

		backends = append(backends, backend.NewBackend(u, startHealthy))
	}

	pool, err := backend.NewBackendPool(backends)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Backend pool created")

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	hc := health.NewHealthChecker(
		ctx,
		pool,
		config.HealthCheckInterval,
		config.HealthCheckTimeout,
	)

	hc.Start()
	defer hc.Stop()

	fmt.Println("Health checker started")

	proxy := internals.NewProxy(pool, config.RequestTimeout)

	serv := http.Server{
		Addr:    config.ListenAddr,
		Handler: proxy,
	}

	errCh := make(chan error, 1)

	go func() {
		errCh <- serv.ListenAndServe()
	}()

	fmt.Printf("Server running %s\n", config.ListenAddr)

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	case <-ctx.Done():
		fmt.Println("Shutting down gracefully")

	}

	stop()

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)

	defer cancel()
	if err := serv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Graceful shutdown failed: %v", err)

		if err := serv.Close(); err != nil {
			log.Printf("Force close failed: %v", err)
		}
	}
}
