package metrics

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	initOnce sync.Once
	global   *Collector
)

// Collector exposes Prometheus metrics for the Sentinel gateway.
type Collector struct {
	RequestsTotal        *prometheus.CounterVec
	BlockedTotal         *prometheus.CounterVec
	RequestDuration      *prometheus.HistogramVec
	BreakerState         *prometheus.GaugeVec
	RedisUp              prometheus.Gauge
	AdaptiveMultiplier   prometheus.Gauge
	DownstreamLatency    prometheus.Histogram
}

func New() *Collector {
	initOnce.Do(func() {
		global = &Collector{
			RequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
				Name: "sentinel_requests_total",
				Help: "Total HTTP requests processed by the gateway",
			}, []string{"route", "status"}),
			BlockedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
				Name: "sentinel_blocked_requests_total",
				Help: "Total requests blocked by rate limiting",
			}, []string{"route", "client"}),
			RequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
				Name:    "sentinel_request_duration_seconds",
				Help:    "Request duration in seconds",
				Buckets: prometheus.DefBuckets,
			}, []string{"route"}),
			BreakerState: prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Name: "sentinel_circuit_breaker_state",
				Help: "Circuit breaker state (0=closed, 1=open, 2=half_open)",
			}, []string{"downstream"}),
			RedisUp: prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "sentinel_redis_up",
				Help: "1 if Redis is reachable, 0 otherwise",
			}),
			AdaptiveMultiplier: prometheus.NewGauge(prometheus.GaugeOpts{
				Name: "sentinel_adaptive_multiplier",
				Help: "Current adaptive rate-limit multiplier (1.0 = no tightening)",
			}),
			DownstreamLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
				Name:    "sentinel_downstream_latency_seconds",
				Help:    "Downstream proxy request latency",
				Buckets: prometheus.DefBuckets,
			}),
		}
		prometheus.MustRegister(
			global.RequestsTotal,
			global.BlockedTotal,
			global.RequestDuration,
			global.BreakerState,
			global.RedisUp,
			global.AdaptiveMultiplier,
			global.DownstreamLatency,
		)
	})
	return global
}

func (c *Collector) Handler() http.Handler {
	return promhttp.Handler()
}

func (c *Collector) ObserveRequest(route string, status int, duration time.Duration) {
	c.RequestsTotal.WithLabelValues(route, strconv.Itoa(status)).Inc()
	c.RequestDuration.WithLabelValues(route).Observe(duration.Seconds())
}

func (c *Collector) ObserveBlocked(route, client string) {
	c.BlockedTotal.WithLabelValues(route, client).Inc()
}

func (c *Collector) SetBreakerState(downstream string, state int) {
	c.BreakerState.WithLabelValues(downstream).Set(float64(state))
}

func (c *Collector) SetRedisUp(up bool) {
	if up {
		c.RedisUp.Set(1)
	} else {
		c.RedisUp.Set(0)
	}
}

func (c *Collector) SetAdaptiveMultiplier(m float64) {
	c.AdaptiveMultiplier.Set(m)
}

func (c *Collector) ObserveDownstreamLatency(d time.Duration) {
	c.DownstreamLatency.Observe(d.Seconds())
}
