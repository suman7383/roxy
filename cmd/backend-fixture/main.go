// backend-fixture is a small HTTP server that simulates multiple backend servers.
// It is used to test the proxy server.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"
)

type backendServer struct {
	id      int
	healthy atomic.Bool
	server  *http.Server
	url     string
}

func newBackend(id int) (*backendServer, error) {
	b := &backendServer{id: id}
	b.healthy.Store(true)

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		log.Println("backend health request", b.id)
		if !b.healthy.Load() {
			http.Error(w, "unhealthy", http.StatusServiceUnavailable)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, "healthy")
	})

	mux.HandleFunc("/admin/health", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("status") {
		case "up":
			b.healthy.Store(true)
			fmt.Fprintf(w, "backend-%d marked healthy\n", b.id)
			log.Println("backend marked healthy", b.id)
		case "down":
			b.healthy.Store(false)
			fmt.Fprintf(w, "backend-%d marked unhealthy\n", b.id)
			log.Println("backend marked unhealthy", b.id)
		default:
			http.Error(w, "use ?status=up or ?status=down", http.StatusBadRequest)
		}
	})

	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		duration := r.URL.Query().Get("duration")
		if duration == "" {
			duration = "3s"
		}

		d, err := time.ParseDuration(duration)
		if err != nil || d <= 0 {
			http.Error(w, "invalid duration", http.StatusBadRequest)
			return
		}

		log.Printf("backend-%d: slow request started (%s)", b.id, d)
		time.Sleep(d)

		log.Printf("backend-%d: slow request finished", b.id)
		fmt.Fprintf(w, "backend-%d finished slow request after %s\n", b.id, d)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("backend-%d: %s %s", b.id, r.Method, r.URL.Path)
		fmt.Fprintf(w, "Hello from backend-%d\n", b.id)
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	b.url = "http://" + ln.Addr().String()
	b.server = &http.Server{Handler: mux}

	go func() {
		log.Printf("backend-%d listening at %s", b.id, b.url)
		if err := b.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("backend-%d server error: %v", b.id, err)
		}
	}()

	return b, nil
}

func main() {
	n := flag.Int("n", 3, "number of backends to start")
	flag.Parse()

	if *n < 1 {
		log.Fatal("number of backends must be >= 1")
	}

	backends := make([]*backendServer, 0, *n)

	for i := 1; i <= *n; i++ {
		b, err := newBackend(i)
		if err != nil {
			log.Fatalf("failed to start backend-%d: %v", i, err)
		}
		backends = append(backends, b)
	}

	fmt.Println()
	fmt.Println("Backends started. Copy these URLs into config.yaml:")
	fmt.Println()

	for _, b := range backends {
		fmt.Printf("  - %q\n", b.url)
	}

	fmt.Println()
	fmt.Println("Press Ctrl+C to stop the fixture backends.")

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	<-ctx.Done()
	fmt.Println("\nStopping fixture backends...")

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	for _, b := range backends {
		if err := b.server.Shutdown(shutdownCtx); err != nil {
			log.Printf("backend-%d shutdown failed: %v", b.id, err)
			_ = b.server.Close()
		}
	}

	fmt.Println("Fixture backends stopped.")
}
