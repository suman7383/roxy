package backend

import (
	"errors"
	"net/url"
	"sync"
	"testing"
)

func TestBackendPool_RoundRobin(t *testing.T) {
	backends := []*Backend{
		NewBackend(&url.URL{Scheme: "http", Host: "backend1"}, true),
		NewBackend(&url.URL{Scheme: "http", Host: "backend2"}, true),
		NewBackend(&url.URL{Scheme: "http", Host: "backend3"}, true),
	}

	pool, err := NewBackendPool(backends)
	if err != nil {
		t.Fatalf("unexpected error creating backend pool: %v", err)
	}

	for i := range 2 * len(backends) {
		backend, err := pool.Next()
		if err != nil {
			t.Fatalf("unexpected error getting next backend: %v", err)
		}

		expectedHost := backends[(i+1)%len(backends)].URL.Host
		if backend.URL.Host != expectedHost {
			t.Errorf("expected backend host %s, got %s", expectedHost, backend.URL.Host)
		}
	}
}

func TestBackendPool_SkipsUnhealthy(t *testing.T) {
	backends := []*Backend{
		NewBackend(&url.URL{Scheme: "http", Host: "backend1"}, true),
		NewBackend(&url.URL{Scheme: "http", Host: "backend2"}, false),
		NewBackend(&url.URL{Scheme: "http", Host: "backend3"}, true),
	}

	pool, err := NewBackendPool(backends)
	if err != nil {
		t.Fatalf("unexpected error creating backend pool: %v", err)
	}

	for range len(backends) + 1 {
		backend, err := pool.Next()
		if err != nil {
			t.Fatalf("unexpected error getting next backend: %v", err)
		}

		if !backend.Healthy() {
			t.Errorf("expected healthy backend, got unhealthy backend %s", backend.URL.Host)
		}
	}
}

func TestBackendPool_NoHealthyBackend(t *testing.T) {
	backends := []*Backend{
		NewBackend(&url.URL{Scheme: "http", Host: "backend1"}, false),
		NewBackend(&url.URL{Scheme: "http", Host: "backend2"}, false),
	}

	pool, err := NewBackendPool(backends)
	if err != nil {
		t.Fatalf("unexpected error creating backend pool: %v", err)
	}

	_, err = pool.Next()
	if !errors.Is(err, ErrNoHealthyBackend) {
		t.Fatalf("expected ErrNoHealthyBackend, got %v", err)
	}
}

func TestNewBackendPool_Empty(t *testing.T) {
	backends := []*Backend{}

	_, err := NewBackendPool(backends)
	if !errors.Is(err, ErrEmptyBackendPool) {
		t.Fatalf("expected ErrEmptyBackendPool, got %v", err)
	}
}

// the balance assertion assumes the health state stays stable throughout the test
func TestBackendPool_ConcurrentNext(t *testing.T) {
	backends := []*Backend{
		NewBackend(&url.URL{Scheme: "http", Host: "backend1"}, true),
		NewBackend(&url.URL{Scheme: "http", Host: "backend2"}, true),
		NewBackend(&url.URL{Scheme: "http", Host: "backend3"}, true),
		NewBackend(&url.URL{Scheme: "http", Host: "backend4"}, false),
		NewBackend(&url.URL{Scheme: "http", Host: "backend5"}, true),
	}

	pool, err := NewBackendPool(backends)
	if err != nil {
		t.Fatalf("unexpected error creating backend pool: %v", err)
	}

	const numGoroutines = 100

	var wg sync.WaitGroup
	results := make(chan string, numGoroutines)
	errs := make(chan error, numGoroutines)

	for range numGoroutines {
		wg.Go(func() {
			backend, err := pool.Next()
			if err != nil {
				errs <- err
				return
			}

			if backend == nil {
				errs <- errors.New("Next returned nil backend without error")
				return
			}

			if !backend.Healthy() {
				errs <- errors.New("Next returned unhealthy backend")
				return
			}

			results <- backend.URL.Host
		})
	}

	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("unexpected error from concurrent Next: %v", err)
		}
	}

	// Verify that round-robin selection remains balanced.
	counts := make(map[string]int)
	for host := range results {
		counts[host]++
	}

	minCount := numGoroutines
	maxCount := 0

	for _, backend := range backends {
		if !backend.Healthy() {
			continue
		}

		count := counts[backend.URL.Host]

		if count < minCount {
			minCount = count
		}
		if count > maxCount {
			maxCount = count
		}
	}

	if maxCount-minCount > 1 {
		t.Errorf("expected balanced distribution among healthy backends, got counts: %v", counts)
	}
}
