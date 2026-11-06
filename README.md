# Roxy --- Mini Reverse Proxy

A lightweight HTTP reverse proxy built in Go as a learning project. Roxy
forwards incoming requests to a pool of upstream backends and
distributes traffic using round-robin load balancing.

The V1 implementation focuses on core proxy behavior, backend health
management, request timeouts, and safe process shutdown.

## Features

- HTTP reverse proxy using Go's `net/http` and `httputil.ReverseProxy`
- Multiple upstream backends
- Round-robin load balancing
- Active periodic health checks
  - Unhealthy backends are excluded from routing
  - Recovered backends rejoin the pool
- Configurable upstream request timeout
- Handling for common proxy failures:
  - `502 Bad Gateway` when an upstream request fails
  - `503 Service Unavailable` when no healthy backend is available
  - `504 Gateway Timeout` when an upstream request times out
- Graceful shutdown on `SIGINT` (Ctrl+C) and `SIGTERM`
  - Stops accepting new requests
  - Allows in-flight requests to finish within a shutdown deadline
  - Force-closes remaining connections if the deadline expires
- Structured request logging
- YAML-based configuration
- Local backend fixture utility for manual integration testing

## Requirements

- Go (version compatible with the project's `go.mod`)
- Local HTTP backend services for running the proxy

## Configuration

Roxy reads its configuration from `config.yaml` by default. You can also
pass a config file path as the first command-line argument.

Example:

```yaml
listen_addr: ":8080"

backends:
  - "http://127.0.0.1:9001"
  - "http://127.0.0.1:9002"
  - "http://127.0.0.1:9003"

request_timeout: 5s
health_check_interval: 10s
health_check_timeout: 2s
```

### Configuration fields

Field Description

---

`listen_addr` Address on which the proxy listens, e.g. `:8080`
`backends` List of upstream backend URLs
`request_timeout` Maximum duration for an upstream request
`health_check_interval` Time between active health-check rounds
`health_check_timeout` Timeout for an individual health probe

The configuration loader validates the configuration and applies
defaults for omitted settings.

## Run

Start the proxy using the default configuration file:

```bash
go run .
```

Or provide a config path:

```bash
go run . ./path/to/config.yaml
```

The proxy will listen at the configured `listen_addr`.

Send a request:

```bash
curl http://localhost:8080/
```

## Testing

Run the project's test suite:

```bash
go test ./...
```

Run tests with Go's race detector:

```bash
go test -race ./...
```

## Local backend fixture

The repository includes a small fixture utility that starts a
configurable number of local HTTP backends on dynamically allocated
ports. It prints the backend URLs so they can be copied into the proxy
configuration.

Start three fixture backends:

```bash
go run ./cmd/backend-fixture -n 3
```

Copy the printed URLs into `config.yaml`, then start Roxy in another
terminal.

### Fixture endpoints

---

Endpoint Behavior

---

`/` Returns a response identifying the
backend

`/health` Returns `200 OK` when healthy, or
`503` when unhealthy

`/slow?duration=3s` Delays the response to help test
request timeouts and graceful
shutdown

`/admin/health?status=down` Marks the backend unhealthy

`/admin/health?status=up` Marks the backend healthy again

---

Example: mark a backend unhealthy (replace the port with the fixture's
printed port):

```bash
curl "http://127.0.0.1:41822/admin/health?status=down"
```

After the next health-check round, the proxy should stop routing
requests to that backend.

Recover it:

```bash
curl "http://127.0.0.1:41822/admin/health?status=up"
```

After a successful health check, it should rejoin the backend pool.

### Check round-robin distribution

```bash
for i in {1..9}; do
    curl -s http://localhost:8080/
done
```

With three healthy backends, responses should rotate among them.

## Graceful shutdown

Roxy handles `SIGINT` and `SIGTERM`. Press `Ctrl+C` in the proxy
terminal to initiate graceful shutdown.

The server stops accepting new connections and waits for active requests
to finish, subject to the configured shutdown deadline in the
application. If the deadline expires, remaining connections are
force-closed.

To manually exercise shutdown with an in-flight request:

1.  Start the fixture utility and the proxy.

2.  In another terminal, issue a slow request:

    ```bash
    curl "http://localhost:8080/slow?duration=3s"
    ```

3.  While the request is running, press `Ctrl+C` in the proxy terminal.

4.  Confirm that the request completes before the proxy exits.

To test the shutdown deadline, issue a slow request that exceeds the
shutdown window and press `Ctrl+C` while it is in progress.

## V1 scope

The first version intentionally keeps the proxy focused on the
fundamentals.

**Included:** - HTTP forwarding - Round-robin backend selection - Active
health checks and backend recovery - Request timeout handling - Graceful
shutdown - Structured request logging

**Not included in V1:** - Retries - Circuit breakers - Weighted or
least-connections load balancing - TLS termination - Service discovery -
Dynamic configuration reload

## Project structure

The exact layout may evolve, but the main areas are:

```text
.
├── cmd/
│   └── backend-fixture/  # Local backend fixture utility
├── internals/
│   ├── backend/          # Backend and backend pool
│   ├── config/           # YAML config loading and validation
│   ├── health/           # Active health checking
│   └── ...               # Proxy implementation
├── config.yaml
├── go.mod
└── README.md
```

## Learning goals

This project is intended to explore practical backend and networking
concepts, including:

- Reverse proxying and HTTP request forwarding
- Load balancing
- Health monitoring and backend state
- Context cancellation and deadlines
- Goroutines, channels, and synchronization
- Graceful server lifecycle management
- Integration testing and race detection
