package main

import (
	"encoding/json"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/sentinel-project/sentinel/gateway/internal/adaptive"
	"github.com/sentinel-project/sentinel/gateway/internal/api"
	"github.com/sentinel-project/sentinel/gateway/internal/circuit_breaker"
	sentinelcfg "github.com/sentinel-project/sentinel/gateway/internal/config"
	"github.com/sentinel-project/sentinel/gateway/internal/metrics"
	ratelimitmw "github.com/sentinel-project/sentinel/gateway/internal/middleware"
	sredis "github.com/sentinel-project/sentinel/gateway/internal/redis"
	"github.com/sentinel-project/sentinel/gateway/internal/stats"
)

type app struct {
	cfg            *sentinelcfg.Config
	rateLimit      *ratelimitmw.RateLimit
	circuitBreaker *circuit_breaker.Middleware
	metrics        *metrics.Collector
	stats          *stats.Collector
	admin          *api.AdminHandler
	redis          *sredis.Client
	adaptive       *adaptive.Controller
}

func newApp(cfg *sentinelcfg.Config) (*app, error) {
	var redisClient *sredis.Client
	if cfg.UseRedis {
		failOpen := cfg.RedisFailureMode == "open"
		client, err := sredis.NewClient(cfg.RedisURL, failOpen)
		if err != nil {
			return nil, err
		}
		redisClient = client
	}

	collector := stats.New(cfg.InstanceID)
	factory := ratelimitmw.NewFactory(cfg, redisClient)
	adapt := adaptive.New(cfg.Adaptive)

	return &app{
		cfg:            cfg,
		rateLimit:      ratelimitmw.NewRateLimit(cfg, factory, adapt),
		circuitBreaker: circuit_breaker.NewMiddleware(),
		metrics:        metrics.New(),
		stats:          collector,
		admin:          api.NewAdminHandler(cfg, collector),
		redis:          redisClient,
		adaptive:       adapt,
	}, nil
}

func (a *app) handleHealth(w http.ResponseWriter, r *http.Request) {
	status := "ok"
	redisUp := true
	if a.cfg.UseRedis && a.redis != nil {
		if err := a.redis.Ping(r.Context()); err != nil {
			status = "degraded"
			redisUp = false
		}
	}
	a.metrics.SetRedisUp(redisUp)
	if a.adaptive != nil {
		a.metrics.SetAdaptiveMultiplier(a.adaptive.Multiplier())
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":    status,
		"instance":  a.cfg.InstanceID,
		"region":    a.cfg.Region,
		"algorithm": a.cfg.DefaultLimit.Algorithm,
		"storage":   a.storageMode(),
	})
}

func (a *app) handleTest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"message":  "ok",
		"instance": a.cfg.InstanceID,
		"client":   ratelimitmw.ClientKey(r),
	})
}

func (a *app) storageMode() string {
	if a.cfg.UseRedis {
		return "redis"
	}
	return "memory"
}

func (a *app) metricsHandler() http.Handler {
	return a.metrics.Handler()
}

func (a *app) adminAuth(next http.Handler) http.Handler {
	return a.admin.Auth(next)
}

func (a *app) handleListLimits(w http.ResponseWriter, r *http.Request) {
	a.admin.ListLimits(w, r)
}

func (a *app) handleUpdateLimit(w http.ResponseWriter, r *http.Request) {
	a.admin.UpdateLimit(w, r)
	a.rateLimit.InvalidateCache()
}

func (a *app) handleStats(w http.ResponseWriter, r *http.Request) {
	a.admin.GetStats(w, r)
}

func (a *app) handleStatsWS(w http.ResponseWriter, r *http.Request) {
	a.admin.StatsWebSocket(w, r)
}

func (a *app) rateLimitWithStats(next http.Handler) http.Handler {
	inner := a.rateLimit.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		elapsed := time.Since(start)
		a.metrics.ObserveRequest(r.URL.Path, rec.status, elapsed)
		if a.adaptive != nil {
			a.adaptive.RecordLatency(elapsed)
			a.metrics.SetAdaptiveMultiplier(a.adaptive.Multiplier())
		}
		a.stats.RecordAllowed(r.URL.Path, ratelimitmw.ClientKey(r))
	}))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		inner.ServeHTTP(rec, r)
		if rec.status == http.StatusTooManyRequests {
			client := ratelimitmw.ClientKey(r)
			a.metrics.ObserveBlocked(r.URL.Path, client)
			a.stats.RecordBlocked(r.URL.Path, client)
		}
	})
}

func (a *app) handleProxy(w http.ResponseWriter, r *http.Request) {
	if a.cfg.DownstreamURL == "" {
		http.Error(w, "downstream not configured", http.StatusBadGateway)
		return
	}

	target, err := url.Parse(a.cfg.DownstreamURL)
	if err != nil {
		http.Error(w, "invalid downstream url", http.StatusInternalServerError)
		return
	}

	br := a.circuitBreaker.Get(target.Host)
	a.metrics.SetBreakerState(target.Host, int(br.State()))

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		br.RecordFailure()
		a.metrics.SetBreakerState(target.Host, int(br.State()))
		http.Error(w, "downstream error", http.StatusBadGateway)
	}

	origDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		origDirector(req)
		req.URL.Path = strings.TrimPrefix(r.URL.Path, "/proxy")
	}

	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	start := time.Now()
	proxy.ServeHTTP(rec, r)
	a.metrics.ObserveDownstreamLatency(time.Since(start))

	if rec.status >= 500 {
		br.RecordFailure()
	} else {
		br.RecordSuccess()
	}
	a.metrics.SetBreakerState(target.Host, int(br.State()))
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
