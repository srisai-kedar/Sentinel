package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	sentinelcfg "github.com/sentinel-project/sentinel/gateway/internal/config"
)

func main() {
	cfg, err := sentinelcfg.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// Auto-enable Redis in Docker when explicitly configured.
	if os.Getenv("SENTINEL_USE_REDIS") == "true" {
		cfg.UseRedis = true
	}

	app, err := newApp(cfg)
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	router := buildRouter(app)
	addr := ":" + cfg.Port
	srv := &http.Server{Addr: addr, Handler: router}

	go func() {
		log.Printf("sentinel [%s] listening on %s (redis=%v)", cfg.InstanceID, addr, cfg.UseRedis)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

func buildRouter(a *app) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/health", a.handleHealth)
	r.Handle("/metrics", a.metricsHandler())

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(a.adminAuth)
		r.Get("/limits", a.handleListLimits)
		r.Put("/limits/{route}", a.handleUpdateLimit)
		r.Get("/stats", a.handleStats)
		r.Get("/ws/stats", a.handleStatsWS)
	})

	r.Group(func(r chi.Router) {
		r.Use(a.rateLimitWithStats)
		r.Get("/api/test", a.handleTest)
	})

	r.Group(func(r chi.Router) {
		r.Use(a.rateLimitWithStats)
		r.Use(a.circuitBreaker.Handler)
		r.Handle("/proxy/*", http.HandlerFunc(a.handleProxy))
	})

	return r
}
