# Sentinel — Deployment & Runbook

> Operations guide for local development, Docker Compose, and production-style deployment.

---

## Prerequisites

| Tool | Version |
|------|---------|
| Go | 1.22+ |
| Docker + Compose | v2+ |
| Node.js | 20+ (dashboard dev only) |
| k6 | 0.47+ (load tests) |

---

## Local Development (single gateway, in-memory)

```powershell
cp .env.example .env
cd gateway
go run ./cmd/sentinel
```

| URL | Purpose |
|-----|---------|
| http://localhost:8080/health | Health check |
| http://localhost:8080/api/test | Rate-limited endpoint |
| http://localhost:8080/metrics | Prometheus metrics |

Enable Redis for distributed mode:

```powershell
$env:SENTINEL_USE_REDIS="true"
$env:SENTINEL_REDIS_URL="redis://localhost:6379"
go run ./cmd/sentinel
```

---

## Docker Compose (full stack)

```powershell
docker compose up --build -d
```

| Service | URL | Credentials |
|---------|-----|-------------|
| Load balancer | http://localhost | — |
| Gateway (direct) | http://localhost:8080 | — |
| Dashboard | http://localhost:3000 | — |
| Prometheus | http://localhost:9090 | — |
| Grafana | http://localhost:3001 | admin / admin |
| Redis | localhost:6379 | — |

### Stop / clean

```powershell
docker compose down          # stop containers
docker compose down -v       # stop + remove volumes
make clean                   # alias for down -v
```

---

## Configuration Reference

| Variable | Default | Description |
|----------|---------|-------------|
| `SENTINEL_USE_REDIS` | `false` | Enable Redis-backed distributed limiting |
| `SENTINEL_REDIS_URL` | `redis://localhost:6379` | Redis connection |
| `SENTINEL_REDIS_FAILURE_MODE` | `closed` | `closed` = 503 on Redis failure; `open` = allow |
| `SENTINEL_RATE_LIMIT` | `10` | Token bucket capacity |
| `SENTINEL_REFILL_RATE` | `1` | Tokens per second |
| `SENTINEL_ALGORITHM` | `token_bucket` | Default algorithm |
| `SENTINEL_REGION` | `local` | Key prefix for multi-region readiness |
| `SENTINEL_ADAPTIVE_ENABLED` | `false` | AIMD adaptive rate limiting |
| `SENTINEL_ADAPTIVE_LATENCY_MS` | `100` | Latency threshold to tighten limits |
| `SENTINEL_ADMIN_API_KEY` | `dev-admin-key` | Admin API authentication |

---

## Failure Scenarios

### Redis unavailable (fail-closed)

**Behavior:** Rate-limited routes return `503 Service Unavailable`. `/health` reports `degraded`. `sentinel_redis_up` metric = 0.

**Verify:**

```powershell
make chaos-redis
# Or manually:
docker stop sentinel-redis-1
curl http://localhost/api/test   # expect 503
docker start sentinel-redis-1
```

**Tradeoff:** Fail-closed protects downstream services but denies all traffic when Redis is down. Set `SENTINEL_REDIS_FAILURE_MODE=open` only if availability trumps protection.

### Circuit breaker open

**Behavior:** `/proxy/*` routes return `503` fast-fail without hitting downstream. Breaker recovers via half-open probe after timeout.

### Adaptive limiting active

**Behavior:** When request latency exceeds `SENTINEL_ADAPTIVE_LATENCY_MS`, capacity and refill rate are scaled down (floor: `SENTINEL_ADAPTIVE_MIN_MULTIPLIER`). Recovers slowly when latency normalizes.

Monitor via `sentinel_adaptive_multiplier` in Grafana (1.0 = no tightening).

---

## Load Testing

```powershell
make bench-all        # baseline + rate limit + atomicity
make bench-stats      # capture docker CPU/memory during load
make chaos-redis      # verify fail-closed behavior
```

Results saved to `load-tests/results/`. See [benchmarks.md](benchmarks.md).

---

## Multi-Region Readiness

Sentinel prefixes Redis/memory keys with `SENTINEL_REGION` to avoid cross-region collisions. MVP uses a single Redis instance.

For true multi-region:

1. Deploy independent Sentinel stacks per region (each with own Redis).
2. Set `SENTINEL_REGION=us-east`, `eu-west`, etc.
3. Do **not** share Redis across regions without documented conflict resolution (eventual consistency tradeoffs).

See [ARCHITECTURE.md](ARCHITECTURE.md) Section 17 for design notes.

---

## Production Deployment Notes

For a live demo (Render, Fly.io, Railway):

1. Deploy Redis as a managed service.
2. Deploy 1–3 gateway instances with `SENTINEL_USE_REDIS=true`.
3. Place Nginx or cloud LB in front.
4. Set `SENTINEL_ADMIN_API_KEY` to a strong secret.
5. Expose `/metrics` only to Prometheus scraper (not public internet).

---

## Troubleshooting

| Symptom | Likely cause | Fix |
|---------|-------------|-----|
| All requests 503 | Redis down or unreachable | Check `docker compose ps`, restart redis |
| 429 immediately | Prior test exhausted bucket | Wait for refill or `redis-cli FLUSHDB` |
| Dashboard empty | No traffic yet | Hit `/api/test` a few times |
| Grafana no data | Prometheus not scraping | Check targets at :9090/targets |
| Atomicity test fails | Stale Redis keys | Script auto-generates unique IP; or flush Redis |
