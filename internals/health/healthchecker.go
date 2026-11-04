package health

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/suman7383/roxy/internals/backend"
)

var ErrHealthCheckFailed = errors.New("health check failed")

type HealthChecker struct {
	pool     *backend.BackendPool
	client   *http.Client
	interval time.Duration
	timeout  time.Duration

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewHealthChecker(
	pctx context.Context,
	pool *backend.BackendPool,
	interval,
	timeout time.Duration,
) *HealthChecker {
	ctx, cancel := context.WithCancel(pctx)

	return &HealthChecker{
		pool:     pool,
		client:   &http.Client{},
		interval: interval,
		timeout:  timeout,
		ctx:      ctx,
		cancel:   cancel,
	}
}

func (hc *HealthChecker) Start() {
	hc.wg.Go(func() {
		hc.run()
	})
}

func (hc *HealthChecker) run() {
	// Initial health check before starting the periodic checks
	hc.checkBackendHealth()

	// Start a ticker to periodically check the health of backends
	t := time.NewTicker(hc.interval)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			// Perform health check
			hc.checkBackendHealth()
		case <-hc.ctx.Done():
			return
		}
	}
}

func (hc *HealthChecker) checkBackendHealth() {
	var wg sync.WaitGroup

	for _, be := range hc.pool.Pool() {
		wg.Add(1)
		go func(b *backend.Backend) {
			defer wg.Done()
			probe(hc.ctx, hc.client, b, hc.timeout)
		}(be)
	}
	wg.Wait()
}

// probe checks the health of a single backend by sending an
// HTTP GET request to its /health endpoint. It sets the backend's
// health status based on the response. It uses the provided context
// to handle timeouts and cancellations.
func probe(
	parentCtx context.Context,
	client *http.Client,
	backend *backend.Backend,
	timeout time.Duration,
) {
	healthURL := *backend.URL
	healthURL.Path = "/health"

	ctx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		healthURL.String(),
		nil,
	)
	if err != nil {
		// fmt.Printf("Failed to create request for backend %s: %v\n", backend.URL.Host, err)
		backend.SetHealthy(false)
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		// fmt.Printf("Health check request failed for backend %s: %v\n", backend.URL.Host, err)
		backend.SetHealthy(false)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// TODO: Maybe log the status code for debugging purposes
		// fmt.Printf("Health check failed for backend %s: received status code %d\n", backend.URL.Host, resp.StatusCode)
		backend.SetHealthy(false)
		return
	}

	backend.SetHealthy(true)
}

func (hc *HealthChecker) Stop() {
	hc.cancel()
	hc.wg.Wait()
}
