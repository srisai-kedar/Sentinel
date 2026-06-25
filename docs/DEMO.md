# Sentinel — 2-Minute Demo Script

> Use this outline for portfolio videos, interview walkthroughs, or live demos.

---

## Setup (before recording)

```powershell
docker compose up --build -d
# Wait ~30s for all services healthy
```

Open tabs:
- http://localhost:3000 (dashboard)
- http://localhost:3001 (Grafana — admin/admin)
- Terminal for curl commands

---

## Script (~2 minutes)

### 0:00 — Problem (15 sec)

> "Every system design interview asks you to design a rate limiter. Most candidates describe it on a whiteboard but have never built one that stays correct across multiple servers. Sentinel proves it works — with real benchmarks."

### 0:15 — Architecture (20 sec)

Show README diagram or Grafana dashboard.

> "Three stateless Go gateways behind Nginx, sharing atomic counters in Redis via Lua scripts. No race conditions — I proved it with a 50-request burst test: exactly 10 allowed, 40 blocked."

### 0:35 — Live traffic (30 sec)

```powershell
# Allowed requests
curl http://localhost/api/test

# Hit the limit (repeat quickly or use a loop)
1..15 | ForEach-Object { curl -s -o NUL -w "%{http_code} " http://localhost/api/test }
```

> "First requests pass with `X-RateLimit-Remaining`. Once the bucket empties, we get 429 with a correct `Retry-After` header."

### 1:05 — Dashboard (25 sec)

Switch to dashboard at :3000.

> "The admin dashboard shows live requests per second, allowed vs blocked ratio, and top offenders. I can change limits here without redeploying — watch the config panel update the route in real time."

(Optional: change capacity from 10 to 5 in the config panel.)

### 1:30 — Observability (20 sec)

Switch to Grafana.

> "Prometheus scrapes all three gateway instances. Here is request rate, blocked requests, p99 latency, and Redis health. Under load we sustained 358 requests per second with 4 millisecond p50 latency."

### 1:50 — Correctness proof (10 sec)

> "The benchmark doc has real k6 numbers — not placeholders. 50 concurrent requests from one client: exactly 10 allowed. That's atomic distributed counting."

### 2:00 — Close

> "Four algorithms, circuit breaker, adaptive limiting, fail-closed Redis degradation — all in Go with unit tests and a full Docker stack. Code and docs are on GitHub."

---

## Optional Extensions

- **Fail-closed demo:** `docker stop sentinel-redis-1` → show 503 → restart Redis → recovery
- **Multi-instance:** Show different `instance` fields in `/api/test` responses (load balanced)
- **Adaptive limiting:** Enable `SENTINEL_ADAPTIVE_ENABLED=true`, show multiplier dropping in Grafana

---

## Screenshots Checklist

Capture for README/portfolio:

- [ ] Dashboard — requests/sec chart with traffic
- [ ] Dashboard — top offenders table populated
- [ ] Grafana — request rate + blocked panels
- [ ] Terminal — 429 response with `Retry-After` header
- [ ] k6 summary — atomicity test 10/40 pass

Save to `docs/screenshots/` (placeholders OK if not captured yet).
