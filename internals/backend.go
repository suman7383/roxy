package backend

import (
	"errors"
	"net/url"
	"slices"
	"sync"
	"sync/atomic"
)

type Backend struct {
	URL *url.URL

	mu      sync.RWMutex
	healthy bool
}

func NewBackend(url *url.URL, healthy bool) *Backend {
	return &Backend{
		URL:     url,
		healthy: healthy,
	}
}

func (b *Backend) Healthy() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.healthy
}

type BackendPool struct {
	pool []*Backend

	counter atomic.Uint64
}

var ErrNoHealthyBackend = errors.New("no healthy backend available")
var ErrEmptyBackendPool = errors.New("pool is empty")

func NewBackendPool(pool []*Backend) (*BackendPool, error) {
	if len(pool) == 0 {
		return nil, ErrEmptyBackendPool
	}

	poolCpy := slices.Clone(pool)

	return &BackendPool{
		pool: poolCpy,
	}, nil
}

// Next returns the next healthy backend available to handle the
// request. It returns ErrNoHealthyBackend if no healthy sbackend is
// found in the pool.
//
// Next guarantees that the backend was marked healthy at the moment of selection.
// It does not guarantee that the backend remains healthy throughout the request.
func (bp *BackendPool) Next() (*Backend, error) {
	n := uint64(len(bp.pool))

	// bound the search to at most len(pool) iterations
	for range n {
		// This should be okay if we consistently use counter after Adding and not before it.
		cnt := bp.counter.Add(1)

		idx := cnt % n
		be := bp.pool[idx]

		if be.Healthy() {

			return be, nil
		}

	}

	return nil, ErrNoHealthyBackend
}
