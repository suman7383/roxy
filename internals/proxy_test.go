package internals

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/suman7383/roxy/internals/backend"
)

const healthy = true
const unhealthy = false
const defaultTimeout = 2 * time.Second

func upstreamHandler(t *testing.T, name string, healthy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET method, got %s", r.Method)
		}

		// health path
		if r.URL.Path == "/health" {
			if healthy {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("Upstream is healthy"))
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte("Upstream is unhealthy"))
			}
			return
		}

		w.WriteHeader(http.StatusOK)

		w.Write([]byte("Hello from upstream " + name))
	}
}

func TestProxy_ForwardsRequestToHealthyBackend(t *testing.T) {
	upstream := httptest.NewTestServer(
		t,
		upstreamHandler(t, "A", healthy),
	)

	upstream.Start()
	defer upstream.Close()

	beURL, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("Failed to parse upstream URL: %v", err)
	}

	be := []*backend.Backend{
		backend.NewBackend(beURL, true),
	}

	pool, err := backend.NewBackendPool(be)
	if err != nil {
		t.Fatalf("Failed to create backend pool: %v", err)
	}

	proxy := NewProxy(pool, defaultTimeout)

	proxyServer := httptest.NewTestServer(t, proxy)
	proxyServer.Start()
	defer proxyServer.Close()

	resp, err := http.Get(proxyServer.URL)
	if err != nil {
		t.Fatalf("Failed to make request to proxy: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	if string(body) != "Hello from upstream A" {
		t.Errorf("Expected response body 'Hello from upstream', got '%s'", string(body))
	}
}

func TestProxy_DistributesRequestRoundRobin(t *testing.T) {
	upstreamA := httptest.NewTestServer(
		t,
		upstreamHandler(t, "A", healthy),
	)
	upstreamA.Start()
	defer upstreamA.Close()

	upstreamB := httptest.NewTestServer(
		t,
		upstreamHandler(t, "B", healthy),
	)
	upstreamB.Start()
	defer upstreamB.Close()

	urlA, err := url.Parse(upstreamA.URL)
	if err != nil {
		t.Fatalf("Failed to parse upstream A URL: %v", err)
	}

	urlB, err := url.Parse(upstreamB.URL)
	if err != nil {
		t.Fatalf("Failed to parse upstream B URL: %v", err)
	}

	be := []*backend.Backend{
		backend.NewBackend(urlA, true),
		backend.NewBackend(urlB, true),
	}

	pool, err := backend.NewBackendPool(be)
	if err != nil {
		t.Fatalf("Failed to create backend pool: %v", err)
	}

	proxy := NewProxy(pool, defaultTimeout)

	proxyServer := httptest.NewTestServer(t, proxy)
	proxyServer.Start()
	defer proxyServer.Close()

	// Make 2 * len(be) requests to ensure round-robin distribution
	for i := 1; i <= 2*len(be); i++ {
		resp, err := http.Get(proxyServer.URL)
		if err != nil {
			t.Fatalf("Failed to make request to proxy: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status code 200, got %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}

		expectedResponse := "Hello from upstream A"
		if i%2 == 1 {
			expectedResponse = "Hello from upstream B"
		}

		if string(body) != expectedResponse {
			t.Errorf("Expected response body '%s', got '%s'", expectedResponse, string(body))
		}

		resp.Body.Close()
	}
}

func TestProxy_ReturnsServiceUnavailableWhenNoHealthyBackends(t *testing.T) {
	url, err := url.Parse("http://example.com")
	if err != nil {
		t.Fatalf("Failed to parse URL: %v", err)
	}

	be := []*backend.Backend{
		backend.NewBackend(url, false),
	}

	pool, err := backend.NewBackendPool(be)
	if err != nil {
		t.Fatalf("Failed to create backend pool: %v", err)
	}

	proxy := NewProxy(pool, defaultTimeout)

	proxyServer := httptest.NewTestServer(t, proxy)
	proxyServer.Start()
	defer proxyServer.Close()

	resp, err := http.Get(proxyServer.URL)
	if err != nil {
		t.Fatalf("Failed to make request to proxy: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected status code 503, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	expectedResponse := backend.ErrNoHealthyBackend.Error() + "\n"
	if string(body) != expectedResponse {
		t.Errorf("Expected response body '%s', got '%s'", expectedResponse, string(body))
	}
}

func TestProxy_ForwardsUpstreamInternalServerError(t *testing.T) {
	expectedResponse := "Upstream internal server error"

	upstream := httptest.NewTestServer(
		t,
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(expectedResponse))
		}),
	)

	upstream.Start()
	defer upstream.Close()

	url, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("Failed to parse upstream URL: %v", err)
	}

	be := []*backend.Backend{
		backend.NewBackend(url, true),
	}

	pool, err := backend.NewBackendPool(be)
	if err != nil {
		t.Fatalf("Failed to create backend pool: %v", err)
	}

	proxy := NewProxy(pool, defaultTimeout)

	proxyServer := httptest.NewTestServer(t, proxy)
	proxyServer.Start()
	defer proxyServer.Close()

	resp, err := http.Get(proxyServer.URL)
	if err != nil {
		t.Fatalf("Failed to make request to proxy: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("Expected status code 500, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	if string(body) != expectedResponse {
		t.Errorf("Expected response body '%s', got '%s'", expectedResponse, string(body))
	}
}

func TestProxy_ReturnsBadGatewayWhenUpstreamUnreachable(t *testing.T) {
	upstream := httptest.NewTestServer(
		t,
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)

	upstream.Start()

	url, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("Failed to parse upstream URL: %v", err)
	}

	// Close the upstream server to simulate it being unreachable
	upstream.Close()

	be := []*backend.Backend{
		backend.NewBackend(url, true),
	}

	pool, err := backend.NewBackendPool(be)
	if err != nil {
		t.Fatalf("Failed to create backend pool: %v", err)
	}

	proxy := NewProxy(pool, defaultTimeout)

	proxyServer := httptest.NewTestServer(t, proxy)
	proxyServer.Start()
	defer proxyServer.Close()

	resp, err := http.Get(proxyServer.URL)
	if err != nil {
		t.Fatalf("Failed to make request to proxy: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("Expected status code 502, got %d", resp.StatusCode)
	}
}

func TestProxy_ReturnsGatewayTimeoutWhenUpstreamExceedsTimeout(t *testing.T) {
	upstream := httptest.NewTestServer(
		t,
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t := time.NewTimer(500 * time.Millisecond)

			<-t.C

			w.WriteHeader(http.StatusOK)
		}),
	)

	upstream.Start()
	defer upstream.Close()

	url, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("Failed to parse upstream URL: %v", err)
	}

	be := []*backend.Backend{
		backend.NewBackend(url, true),
	}

	pool, err := backend.NewBackendPool(be)
	if err != nil {
		t.Fatalf("Failed to create backend pool: %v", err)
	}

	proxy := NewProxy(pool, 100*time.Millisecond)
	proxyServer := httptest.NewTestServer(t, proxy)
	proxyServer.Start()
	defer proxyServer.Close()

	resp, err := http.Get(proxyServer.URL)
	if err != nil {
		t.Fatalf("Failed to make request to proxy: %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusGatewayTimeout {
		t.Errorf("Expected status code 504, got %d", resp.StatusCode)
	}
}
