package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/heliolytics/api/internal/api"
	"github.com/heliolytics/api/internal/config"
	"github.com/heliolytics/api/internal/store"
)

func main() {
	cfg := config.Load()
	if cfg.SigningSecret == "" {
		log.Fatal("HELIOLYTICS_SIGNING_SECRET is required")
	}
	log.Printf(
		"config: addr=%s signing_secret_len=%d rate_limit=%d/min reparse=%v trust_proxy=%v",
		cfg.Addr, len(cfg.SigningSecret), cfg.RateLimitPerMin, cfg.ReparseEnabled, cfg.TrustProxy,
	)
	log.Printf("config: Flutter app API key must exactly match HELIOLYTICS_SIGNING_SECRET (401 signature_mismatch if not)")
	ctx := context.Background()
	st, err := store.New(ctx, cfg.DBURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer st.Close()

	srv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      api.NewMux(st, cfg),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
	}
	go func() {
		log.Printf("api listening on %s", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
