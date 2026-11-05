package internals

import (
	"context"
	"net/http"
	"net/http/httputil"

	"github.com/suman7383/roxy/internals/backend"
)

type contextKey string

const (
	backendCtxKey contextKey = "backend"
)

type Proxy struct {
	pool *backend.BackendPool
	rp   *httputil.ReverseProxy
}

func NewProxy(pool *backend.BackendPool) *Proxy {
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			// backend is guaranteed to be non-nil here because we set it in ServeHTTP
			// before calling the ReverseProxy.
			backend := pr.Out.Context().Value(backendCtxKey).(*backend.Backend)
			pr.SetURL(backend.URL)
		},
	}
	return &Proxy{
		pool: pool,
		rp:   rp,
	}
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	be, err := p.pool.Next()
	if err != nil {
		http.Error(w, backend.ErrNoHealthyBackend.Error(), http.StatusServiceUnavailable)
		return
	}

	// Validate selected backend
	if be == nil || be.URL == nil {
		http.Error(w, "Invalid backend selected", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	ctx = context.WithValue(ctx, backendCtxKey, be)
	r = r.WithContext(ctx)
	p.rp.ServeHTTP(w, r)
}
