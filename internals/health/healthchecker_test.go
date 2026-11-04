package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/suman7383/roxy/internals/backend"
)

// createTestServer creates a test HTTP server that responds to /health requests.
// The server responds with a 200 OK status if healthy is true,
// and a 500 Internal Server Error if healthy is false.
// The server can also simulate a delay in response if delay is greater than zero.
func createTestServer(
	t *testing.T,
	healthy bool,
	delay time.Duration,
) *httptest.Server {
	t.Helper()

	return httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}

		if delay > 0 {
			timer := time.NewTimer(delay)
			defer timer.Stop()

			select {
			case <-timer.C:
				// Delay elapsed; continue with the health response.
			case <-r.Context().Done():
				// Client canceled the request.
				return
			}
		}

		if healthy {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
}

func TestHealthChecker_MarksBackendHealthy(t *testing.T) {
	// Create a test HTTP server that responds to /health with a 200 OK status
	serv := createTestServer(t, true, 0)
	serv.Start()
	defer serv.Close()

	url, err := url.Parse(serv.URL)
	if err != nil {
		t.Fatalf("Failed to parse server URL: %v", err)
	}

	backends := []*backend.Backend{
		backend.NewBackend(url, false), // Initially unhealthy
	}

	pool, err := backend.NewBackendPool(backends)
	if err != nil {
		t.Fatalf("Failed to create backend pool: %v", err)
	}

	ctx := context.Background()

	hc := NewHealthChecker(ctx, pool, 1*time.Second, 500*time.Millisecond)

	// Directly invoke the health check without starting the periodic checks
	hc.checkBackendHealth()

	// Check if the backend is marked as healthy
	if !backends[0].Healthy() {
		t.Errorf("Expected backend to be healthy, but it is not")
	}

}

func TestHealthChecker_MarksBackendUnhealthy(t *testing.T) {
	serv := createTestServer(t, false, 0)
	serv.Start()
	defer serv.Close()

	url, err := url.Parse(serv.URL)
	if err != nil {
		t.Fatalf("Failed to parse server URL: %v", err)
	}

	backends := []*backend.Backend{
		backend.NewBackend(url, true), // Initially healthy
	}

	pool, err := backend.NewBackendPool(backends)
	if err != nil {
		t.Fatalf("Failed to create backend pool: %v", err)
	}

	ctx := context.Background()

	hc := NewHealthChecker(ctx, pool, 1*time.Second, 500*time.Millisecond)
	hc.checkBackendHealth()

	// Check if the backend is marked as unhealthy
	if backends[0].Healthy() {
		t.Errorf("Expected backend to be unhealthy, but it is not")
	}
}

func TestHealthChecker_ProbeTimeout(t *testing.T) {
	serv := createTestServer(t, true, 500*time.Millisecond) // Simulate a delay
	serv.Start()
	defer serv.Close()

	url, err := url.Parse(serv.URL)
	if err != nil {
		t.Fatalf("Failed to parse server URL: %v", err)
	}

	backends := []*backend.Backend{
		backend.NewBackend(url, true), // Initially healthy
	}

	pool, err := backend.NewBackendPool(backends)
	if err != nil {
		t.Fatalf("Failed to create backend pool: %v", err)
	}

	ctx := context.Background()

	hc := NewHealthChecker(ctx, pool, 1*time.Second, 50*time.Millisecond)
	hc.checkBackendHealth()

	// Check if the backend is marked as unhealthy
	if backends[0].Healthy() {
		t.Errorf("Expected backend to be unhealthy, but it is not")
	}
}

func TestHealthChecker_BackendRecovery(t *testing.T) {
	var statusCode atomic.Int32
	statusCode.Store(http.StatusInternalServerError)

	serv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}

		if statusCode.Load() == http.StatusOK {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))

	serv.Start()
	defer serv.Close()

	url, err := url.Parse(serv.URL)
	if err != nil {
		t.Fatalf("Failed to parse server URL: %v", err)
	}

	backends := []*backend.Backend{
		backend.NewBackend(url, true), // Initially healthy
	}

	pool, err := backend.NewBackendPool(backends)
	if err != nil {
		t.Fatalf("Failed to create backend pool: %v", err)
	}

	ctx := context.Background()

	hc := NewHealthChecker(ctx, pool, 1*time.Second, 500*time.Millisecond)

	// First, mark the backend as unhealthy
	hc.checkBackendHealth()

	if backends[0].Healthy() {
		t.Errorf("Expected backend to be unhealthy, but it is not")
	}

	// Now, mark the backend as healthy
	statusCode.Store(http.StatusOK)
	hc.checkBackendHealth()

	if !backends[0].Healthy() {
		t.Errorf("Expected backend to be healthy, but it is not")
	}
}

func TestHealthChecker_Stop(t *testing.T) {
	reqStarted := make(chan struct{})
	resCancelled := make(chan struct{})

	serv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(reqStarted)

		<-r.Context().Done()
		close(resCancelled)
	}))

	serv.Start()
	defer serv.Close()

	url, err := url.Parse(serv.URL)
	if err != nil {
		t.Fatalf("Failed to parse server URL: %v", err)
	}

	backends := []*backend.Backend{
		backend.NewBackend(url, true),
	}

	pool, err := backend.NewBackendPool(backends)
	if err != nil {
		t.Fatalf("Failed to create backend pool: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hc := NewHealthChecker(ctx, pool, 1*time.Second, 500*time.Millisecond)

	hc.Start()

	// Wait for the health check request to start
	<-reqStarted

	stopDone := make(chan struct{})
	go func() {
		hc.Stop()
		close(stopDone)
	}()

	select {
	case <-stopDone:
		// Success: Stop completed
	case <-time.After(1 * time.Second):
		t.Errorf("Expected health checker to stop, but it did not")
	}

	select {
	case <-resCancelled:
		// Success: the active request was cancelled.
	case <-time.After(1 * time.Second):
		t.Errorf("Expected health check request to be cancelled")
	}

}
