# Sentinel — Architecture

> Engineering design document for the Sentinel distributed rate limiter and API gateway.

**Status:** Complete (Phases 0–9) — distributed rate limiter, 4 algorithms, Redis Lua atomicity, circuit breaker, adaptive limiting, observability, admin dashboard, k6 benchmarks, and production runbooks.

---

## 1. Problem Statement

Backend services must be protected from traffic spikes, buggy clients, and abusive actors by enforcing per-user / per-IP / per-API-key request quotas. When the gateway runs as multiple horizontally-scaled instances behind a load balancer, rate limiting must remain **correct**: no double-counting, no race conditions, and consistent decisions regardless of which instance handles a request.

Most interview candidates can whiteboard a rate limiter. Few have implemented one that stays correct under real concurrent load across N nodes. Sentinel exists to prove that understanding with working code and benchmark numbers.

---

## 2. Goals

| Goal | Description |
|------|-------------|
| **Correctness under concurrency** | Atomic distributed counting via Redis + Lua scripts; no read-modify-write races across instances |
| **Four interchangeable algorithms** | Token Bucket, Sliding Window Log, Sliding Window Counter, Leaky Bucket — swappable via config |
| **Horizontal scalability** | Stateless gateway instances; shared Redis state; Nginx load balancing |
| **Graceful rejection** | HTTP 429 with accurate `Retry-After` headers |
| **Downstream protection** | Circuit breaker to prevent cascading failures |
| **Observability** | Prometheus metrics, Grafana dashboards, live admin UI |
| **Runtime configuration** | Update per-route limits via admin API without redeploying |
| **Interview defensibility** | Every file explainable line-by-line; complexity documented per algorithm |

---

## 3. Non-Goals

- Full multi-region Redis replication with conflict resolution (readiness only — see Section 17)
- Cost-based / points-based limiting (future work)
- Postgres-backed config persistence (optional, not implemented)
- Enterprise auth beyond API key (MVP)
- Kubernetes deployment (Docker Compose sufficient for portfolio)

---

## 4. System Architecture

```
                    ┌─────────────────┐
                    │  Nginx (LB)     │
                    │  :80            │
                    └────────┬────────┘
                             │
           ┌─────────────────┼─────────────────┐
           │                 │                 │
    ┌──────▼──────┐   ┌──────▼──────┐   ┌──────▼──────┐
    │ Gateway-1   │   │ Gateway-2   │   │ Gateway-3   │
    │ (Go + chi)  │   │ (Go + chi)  │   │ (Go + chi)  │
    │ stateless   │   │ stateless   │   │ stateless   │
    └──────┬──────┘   └──────┬──────┘   └──────┬──────┘
           │                 │                 │
           └─────────────────┼─────────────────┘
                             │
                    ┌────────▼────────┐
                    │  Redis (single) │
                    │  Lua scripts    │
                    └────────┬────────┘
                             │
                    ┌────────▼────────┐
                    │  Downstream     │
                    │  Services       │
                    └─────────────────┘

    ┌─────────────┐   ┌─────────────┐   ┌─────────────┐
    │ Prometheus  │   │  Grafana    │   │  Dashboard  │
    │ (metrics)   │   │ (visualize) │   │ (React UI)  │
    └─────────────┘   └─────────────┘   └─────────────┘
```

### Component Responsibilities

| Component | Role |
|-----------|------|
| **Nginx** | Round-robin / least-conn load balancing across gateway instances |
| **Gateway (Go + chi)** | HTTP proxy, rate-limit middleware, circuit breaker, admin API, metrics export |
| **Redis** | Shared atomic state for all rate-limit counters (single instance for MVP) |
| **Lua scripts** | Atomic check-and-increment per algorithm — one script execution = one atomic operation |
| **Prometheus + Grafana** | Request rates, latency histograms, blocked-request counters |
| **React Dashboard** | Live charts, top offenders, runtime config panel via WebSocket |

---

## 5. Why Naive Counting Breaks Under Concurrency

Consider two gateway instances handling requests for the same client at the same time:

```
Instance A: READ count = 9   (limit is 10)
Instance B: READ count = 9   (limit is 10)
Instance A: CHECK 9 < 10 → allow
Instance B: CHECK 9 < 10 → allow
Instance A: INCREMENT → count = 10
Instance B: INCREMENT → count = 11   ← limit exceeded, both were allowed
```

This is a classic **lost update** race. Application-level mutexes don't help across processes. Even Redis `INCR` alone isn't enough — you need the read, check, and increment to happen as a single atomic unit.

### How Lua Scripts Fix This

Redis executes Lua scripts atomically — no other command interleaves mid-script. The pattern:

```
EVAL token_bucket.lua 1 sentinel:token_bucket:client:abc 100 10 1718841600000
```

Inside the script:

1. Read current token count and last refill timestamp
2. Compute refill based on elapsed time
3. Check if tokens remain
4. Decrement if allowed
5. Return `{ allowed, retry_after_ms, remaining }`

All in one round-trip, one atomic operation. This is why we chose Lua over app-level locks or separate GET/SET commands.

### Why Not CAS (Compare-And-Swap)?

Redis doesn't expose CAS directly on arbitrary data structures. `WATCH`/`MULTI`/`EXEC` optimistic transactions work but retry under contention. Lua is simpler, single-round-trip, and is the industry-standard pattern for rate limiting in Redis (used by GitHub, Stripe, and others).

---

## 6. Rate-Limiting Algorithms

All algorithms implement a common Go interface:

```go
type RateLimiter interface {
    Allow(ctx context.Context, key string) (allowed bool, retryAfter time.Duration, err error)
}
```

Selection is config-driven (`SENTINEL_ALGORITHM=token_bucket|sliding_window_log|...`).

### 6.1 Token Bucket

- **Mechanism:** Bucket holds N tokens; refills at rate R/sec; each request consumes 1 token.
- **Burst:** Allows bursts up to bucket capacity.
- **Complexity:** O(1) amortized per `Allow` (lazy refill on access).
- **Use when:** You want to allow short bursts while enforcing average rate.

### 6.2 Sliding Window Log

- **Mechanism:** Store timestamp of every request in a sorted set (Redis) or deque (in-memory). Count entries within `[now - window, now]`.
- **Precision:** Exact — every request timestamp is tracked.
- **Complexity:** O(1) amortized with lazy cleanup; O(k) worst case where k = requests in window.
- **Memory:** O(k) per client key — expensive at high RPS.
- **Use when:** Precision matters and traffic per key is moderate.

### 6.3 Sliding Window Counter

- **Mechanism:** Maintain two fixed-window counters (current + previous). Estimate sliding count: `prev_count × (1 - elapsed/window) + curr_count`.
- **Precision:** Approximate — blends two fixed windows.
- **Complexity:** O(1) time and space per key.
- **Use when:** High throughput, many keys, and ~approximate limiting is acceptable (common in production).

### Tradeoff Summary

| | Sliding Window Log | Sliding Window Counter |
|---|---|---|
| **Precision** | Exact | ~Approximate |
| **Memory per key** | O(requests in window) | O(1) |
| **Best for** | Low-volume, strict limits | High-volume, many clients |

---

## 7. Redis Key Design

```
sentinel:{algorithm}:{scope}:{identifier}
```

Examples:

```
sentinel:token_bucket:route:/api/users:client:192.168.1.1
sentinel:sliding_window_log:apikey:sk-abc123:route:/api/orders
```

Long identifiers are hashed (FNV-1a) to keep keys under Redis memory efficiency thresholds. Hashing strategy is documented in code comments for interview discussion.

**MVP:** Single Redis instance. Sharding via consistent hashing is a Phase 8 consideration if key volume demands it.

---

## 8. Fail-Closed vs Fail-Open

| Mode | Behavior when Redis is down |
|------|----------------------------|
| **Fail-open** | Allow all traffic — prioritizes availability |
| **Fail-closed** ✓ | Deny traffic (503/429) — prioritizes protection |

### Sentinel's Choice: Fail-Closed (default)

When Redis is unreachable, the gateway **cannot verify quota state**. Allowing traffic would:

1. Defeat the purpose of rate limiting during the exact scenario (overload) where it's needed most
2. Risk cascading failures to downstream services
3. Create a false sense of protection

**Default:** `SENTINEL_REDIS_FAILURE_MODE=closed` → return `503 Service Unavailable`.

**Configurable:** Set to `open` for demos or environments where availability trumps protection. Documented so you can discuss the tradeoff in interviews.

---

## 9. Circuit Breaker

Protects configured downstream services from cascading failure.

```
Closed ──(failure threshold exceeded)──► Open
  ▲                                      │
  │                                      │ (timeout elapsed)
  │                                      ▼
  └────(probe succeeds)──────────── Half-Open
```

- **Closed:** Requests pass through; failures are counted.
- **Open:** Requests fail fast without hitting downstream.
- **Half-Open:** One probe request allowed; success → Closed, failure → Open.

Implemented in `gateway/internal/circuit_breaker/` (Phase 4).

---

## 10. Request Flow

```
Client Request
    │
    ▼
Nginx (load balance)
    │
    ▼
Gateway Middleware Chain
    ├── 1. Extract identity (IP, API key, route)
    ├── 2. Rate limit check (Redis Lua script)
    │       ├── Allowed → continue
    │       └── Denied  → 429 + Retry-After
    ├── 3. Circuit breaker check
    │       ├── Open → 503 fast-fail
    │       └── Closed → proxy to downstream
    └── 4. Record metrics (Prometheus)
    │
    ▼
Downstream Service
```

---

## 11. Admin API & Dashboard

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/v1/limits` | GET | List current rate-limit configs |
| `/api/v1/limits/{route}` | PUT | Update limit for a route at runtime |
| `/api/v1/stats` | GET | Snapshot of traffic stats |
| `/ws/stats` | WebSocket | Live feed for dashboard |

Admin endpoints protected by `SENTINEL_ADMIN_API_KEY` (Phase 5).

Dashboard (Phase 6): dark-mode developer-tool aesthetic with Recharts — requests/sec line chart, allowed vs blocked ratio, top offenders table, live config panel.

---

## 12. Alternatives Considered

| Alternative | Why Not (for MVP) |
|-------------|-------------------|
| **In-process only (no Redis)** | Cannot scale horizontally; each instance has independent counters |
| **Redis INCR without Lua** | Race condition on read-check-increment pattern |
| **PostgreSQL counters** | Too slow for per-request counting at gateway latency requirements |
| **Envoy rate limiting** | Less educational; harder to explain algorithm internals in interviews |
| **Node.js / FastAPI gateway** | Go's concurrency model and lower overhead better suit gateway workloads |
| **Redis Cluster (multi-node)** | Unnecessary complexity for MVP; single instance proves correctness |

---

## 13. Tradeoffs Chosen

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Language | Go + chi | Concurrency, performance, industry expectation for infra |
| State store | Single Redis + Lua | Atomic ops, proven pattern, simple MVP topology |
| Failure mode | Fail-closed | Gateway's job is protection, not availability-at-all-costs |
| Algorithm default | Token Bucket | Simplest to implement and reason about; good burst behavior |
| Config hot-reload | In-memory + admin API | Postgres persistence deferred to Phase 5+ |
| Load balancer | Nginx | Simple, well-understood, easy to demo |
| Observability | Prometheus + Grafana | Industry standard; Go has excellent client library |

---

## 14. DSA Concepts Showcased

Each algorithm file will include documented complexity analysis and data structure choices:

| Concept | Where |
|---------|-------|
| Sliding window (deque / two-pointer) | `sliding_window_log.go` |
| Queue-based token refill | `token_bucket.go` |
| Hashing for bucket keys | `config.go` / key builder |
| Heap for lazy expiry cleanup | `sliding_window_log.go` |
| Amortized complexity reasoning | Comment block in each algorithm file |
| Atomic ops vs mutex | `redis/client.go` + Lua scripts |

---

## 15. Phased Implementation Plan

| Phase | Deliverable |
|-------|-------------|
| 0 | Scaffold, docs, Docker Compose placeholders |
| 1 | Single-instance gateway, in-memory Token Bucket |
| 2 | Redis + Lua, 3 instances behind Nginx |
| 3 | Remaining 3 algorithms + unit tests |
| 4 | Circuit breaker |
| 5 | Prometheus/Grafana + admin REST API |
| 6 | React dashboard |
| 7 | k6 load tests + real benchmark numbers |
| 7 | k6 load tests + real benchmark numbers |
| 8 | Adaptive limiting, chaos tests, multi-region readiness, observability |
| 9 | Final docs, resume bullets, demo script, deployment runbook |

---

## 17. Adaptive Rate Limiting (Phase 8)

Sentinel supports optional **AIMD-style** adaptive limiting (`SENTINEL_ADAPTIVE_ENABLED=true`).

```
Latency > threshold  →  multiplier *= 0.5  (multiplicative decrease)
Latency < threshold/2 →  multiplier += 0.1 (additive increase, cap 1.0)
```

**Tradeoff:** Protects downstream during latency spikes but may throttle legitimate traffic during transient slowness. A floor (`SENTINEL_ADAPTIVE_MIN_MULTIPLIER`, default 0.25) ensures limits never drop to zero.

**Safe fallback:** When disabled (default), base configured limits apply unchanged.

Implementation: `gateway/internal/adaptive/controller.go` — scales effective capacity and refill rate before each rate-limit check.

---

## 18. Multi-Region Readiness (Phase 8)

MVP uses a **single Redis instance**. Multi-region readiness is config-level only:

- `SENTINEL_REGION` prefixes all bucket keys (`region:route:client`)
- Each region should run an independent Redis — shared cross-region Redis requires documented conflict handling (not implemented)

**Recommended production pattern:** One Sentinel stack per region, region-scoped keys, global traffic routed via geo-DNS to nearest stack.

---

## 19. Chaos & Resilience (Phase 8)

| Failure | Behavior | Verified by |
|---------|----------|-------------|
| Redis down (fail-closed) | `503` on rate-limited routes; `/health` = degraded | Unit test + `make chaos-redis` |
| Redis down (fail-open) | Traffic allowed (configurable, not default) | Config only |
| Circuit breaker open | `503` fast-fail on `/proxy/*` | Unit tests |
| Adaptive spike | Limits tighten automatically | `adaptive` package tests |

See [DEPLOYMENT.md](DEPLOYMENT.md) for runbook procedures.

---

## 20. Security Notes

- Admin API key required for config endpoints
- No PII stored in Redis keys (hashed identifiers)
- Rate-limit keys expire via TTL to prevent unbounded memory growth
- Nginx forwards `X-Real-IP` for client identification behind load balancer
