# Sentinel

**A distributed rate limiter and API gateway** — horizontally scalable, multi-algorithm, proven correct under real concurrent load.

Built to answer the interview question *"design a rate limiter"* with working code and benchmark numbers, not just a whiteboard diagram.

```
                    ┌─────────────────┐
                    │  Nginx (LB)     │
                    └────────┬────────┘
                             │
           ┌─────────────────┼─────────────────┐
           │                 │                 │
    ┌──────▼──────┐   ┌──────▼──────┐   ┌──────▼──────┐
    │ Gateway-1   │   │ Gateway-2   │   │ Gateway-3   │
    │  (Go+chi)   │   │  (Go+chi)   │   │  (Go+chi)   │
    └──────┬──────┘   └──────┬──────┘   └──────┬──────┘
           └─────────────────┼─────────────────┘
                             │
                    ┌────────▼────────┐
                    │  Redis + Lua    │
                    │  (atomic ops)   │
                    └────────┬────────┘
                             │
              ┌──────────────┼──────────────┐
              │              │              │
       ┌──────▼─────┐ ┌─────▼─────┐ ┌──────▼──────┐
       │ Prometheus │ │  Grafana  │ │  Dashboard  │
       └────────────┘ └───────────┘ └─────────────┘
```

---

## The Problem

Backend services need protection from traffic spikes, buggy clients, and abusive actors. Rate limits must stay **correct** when the gateway runs as N stateless instances behind a load balancer — no double-counting, no race conditions.

Most candidates can describe this on a whiteboard. Sentinel proves it works.

---

## Benchmarks

Measured with k6 against the live Docker Compose stack (3 gateways, Nginx, Redis). Full methodology in [docs/benchmarks.md](docs/benchmarks.md).

| Metric | Result |
|--------|--------|
| Sustained RPS (`/health`, 3 gateways) | **358 req/s** |
| Sustained RPS (`/api/test`, rate-limited) | **125 req/s** |
| p50 latency (rate-limited) | **4.0 ms** |
| p95 latency (rate-limited) | **7.7 ms** |
| p99 latency (rate-limited) | **16.9 ms** |
| Redis atomicity (50 concurrent, cap=10) | **10 allowed / 40 blocked** |
| Server error rate | **0.00%** |

---

## Features

| Feature | Status |
|---------|--------|
| Token Bucket, Sliding Window Log/Counter, Leaky Bucket | Implemented |
| Redis + Lua atomic distributed counting | Implemented |
| 3 gateway instances behind Nginx | Implemented |
| Per-route, per-client rate limits | Implemented |
| 429 + `Retry-After` headers | Implemented |
| Fail-closed when Redis unavailable | Implemented |
| Circuit breaker (downstream protection) | Implemented |
| AIMD adaptive rate limiting | Implemented (opt-in) |
| Prometheus + Grafana observability | Implemented |
| Admin REST API + WebSocket stats | Implemented |
| React dark-mode dashboard | Implemented |
| k6 load tests with real numbers | Implemented |
| Multi-region key prefix readiness | Implemented |

---

## Quick Start

### Docker Compose (recommended)

```powershell
git clone <repo-url>
cd Sentinel
cp .env.example .env
docker compose up --build -d
```

| Service | URL |
|---------|-----|
| API (load balanced) | http://localhost |
| Gateway direct | http://localhost:8080 |
| Admin dashboard | http://localhost:3000 |
| Prometheus | http://localhost:9090 |
| Grafana | http://localhost:3001 (admin / admin) |

```powershell
curl http://localhost/health
curl http://localhost/api/test
```

### Local (single instance, in-memory)

```powershell
cd gateway
go run ./cmd/sentinel
```

### Tests & benchmarks

```powershell
cd gateway && go test ./... -count=1
make bench-all          # k6 load tests
make chaos-redis        # verify fail-closed behavior
```

---

## API Endpoints

| Endpoint | Auth | Description |
|----------|------|-------------|
| `GET /health` | — | Health + Redis status |
| `GET /api/test` | — | Rate-limited test endpoint |
| `GET /metrics` | — | Prometheus metrics |
| `GET /api/v1/limits` | Admin key | List rate limits |
| `PUT /api/v1/limits/{route}` | Admin key | Update limits at runtime |
| `GET /api/v1/stats` | Admin key | Traffic snapshot |
| `GET /api/v1/ws/stats` | Admin key | Live WebSocket feed |
| `/proxy/*` | — | Downstream proxy (circuit breaker) |

Admin header: `X-Admin-Key: dev-admin-key`

---

## Configuration

Key environment variables (see [.env.example](.env.example)):

| Variable | Default | Description |
|----------|---------|-------------|
| `SENTINEL_USE_REDIS` | `false` | Distributed Redis limiting |
| `SENTINEL_RATE_LIMIT` | `10` | Bucket capacity |
| `SENTINEL_REFILL_RATE` | `1` | Tokens/sec |
| `SENTINEL_REDIS_FAILURE_MODE` | `closed` | Deny on Redis failure |
| `SENTINEL_ADAPTIVE_ENABLED` | `false` | AIMD adaptive limiting |
| `SENTINEL_REGION` | `local` | Multi-region key prefix |

---

## What This Demonstrates

- **Distributed systems correctness** — Lua atomicity across N instances; 10/50 burst test passes
- **Algorithm design** — 4 interchangeable strategies with documented complexity
- **Production patterns** — circuit breaker, adaptive limiting, fail-closed, observability
- **Full-stack delivery** — Go gateway, React dashboard, k6 benchmarks, Docker Compose
- **Interview readiness** — every design decision documented with tradeoffs

---

## Repository Structure

```
sentinel/
├── gateway/              # Go API gateway
│   ├── cmd/sentinel/     # Entrypoint
│   └── internal/
│       ├── algorithms/   # 4 rate-limit strategies
│       ├── adaptive/     # AIMD adaptive controller
│       ├── middleware/   # Rate-limit middleware
│       ├── circuit_breaker/
│       ├── redis/        # Client + embedded Lua
│       ├── metrics/      # Prometheus
│       └── api/          # Admin REST + WebSocket
├── dashboard/            # React admin UI
├── load-tests/           # k6 scenarios + results
├── infra/                # Nginx, Prometheus, Grafana
└── docs/
    ├── ARCHITECTURE.md
    ├── benchmarks.md
    ├── DEPLOYMENT.md
    └── DEMO.md
```

---

## Documentation

- [Architecture & Design Decisions](docs/ARCHITECTURE.md)
- [Benchmarks (real k6 numbers)](docs/benchmarks.md)
- [Deployment & Runbook](docs/DEPLOYMENT.md)
- [2-Minute Demo Script](docs/DEMO.md)

---

## Tech Stack

Go · chi · Redis · Lua · Nginx · Prometheus · Grafana · React · k6 · Docker Compose

---

## License

MIT
