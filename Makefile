.PHONY: up down build test bench bench-all logs clean gateway dashboard

up:
	docker compose up --build -d

down:
	docker compose down

build:
	cd gateway && go build -o bin/sentinel ./cmd/sentinel

test:
	cd gateway && go test ./... -count=1

test-redis:
	cd gateway && SENTINEL_REDIS_URL=redis://localhost:6379 go test ./internal/algorithms -run Redis -v

bench:
	k6 run load-tests/scenarios/baseline.js

bench-all:
	k6 run load-tests/scenarios/baseline.js -e BASE_URL=http://localhost
	k6 run load-tests/scenarios/rate_limit.js -e BASE_URL=http://localhost
	k6 run load-tests/scenarios/redis_atomicity.js -e BASE_URL=http://localhost

bench-stats:
	powershell -File load-tests/scripts/capture-docker-stats.ps1 -DurationSec 120

chaos-redis:
	powershell -File load-tests/scripts/chaos-redis-down.ps1

logs:
	docker compose logs -f gateway-1 gateway-2 gateway-3

gateway:
	cd gateway && go run ./cmd/sentinel

dashboard:
	cd dashboard && npm install && npm run dev

clean:
	docker compose down -v
