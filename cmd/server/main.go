package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Anshuman115/heliolytics/internal/api"
	"github.com/Anshuman115/heliolytics/internal/config"
	"github.com/Anshuman115/heliolytics/internal/ingest"
	"github.com/Anshuman115/heliolytics/internal/store"
)

// Set with -ldflags '-X main.commit=... -X main.builtAt=...'.
var commit, builtAt string

const (
	healthEndpoint = "http://127.0.0.1:8080/health"

	healthcheckTimeout = 2 * time.Second
	noncePurgeInterval = 10 * time.Minute
	shutdownTimeout    = 30 * time.Second

	readHeaderTimeout = 5 * time.Second
	readTimeout       = 95 * time.Second
	writeTimeout      = 95 * time.Second
	idleTimeout       = 60 * time.Second
	maxHeaderBytes    = 16 << 10
)

var probeTarget = healthEndpoint

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

func run(args []string, stderr io.Writer) int {
	logger := log.New(stderr, "", log.LstdFlags)

	flags := flag.NewFlagSet("server", flag.ContinueOnError)
	flags.SetOutput(stderr)
	healthcheck := flags.Bool("healthcheck", false, "probe the local health endpoint and exit")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() > 0 {
		logger.Printf("unexpected positional arguments: %d", flags.NArg())
		return 2
	}
	if *healthcheck {
		return healthcheckExit(logger)
	}

	cfg, err := config.Load()
	if err != nil {
		logger.Printf("config: %v", err)
		return 1
	}
	applyBuildInfo(&cfg)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return serve(ctx, cfg, logger)
}

func applyBuildInfo(cfg *config.Config) {
	cfg.Commit = commit
	cfg.BuiltAt = builtAt
}

func serve(ctx context.Context, cfg config.Config, logger *log.Logger) int {
	st, err := store.New(ctx, cfg.DBURL)
	if err != nil {
		logger.Printf("store: %v", err)
		return 1
	}
	defer st.Close()

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           newHandler(st, cfg),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
	}

	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		logger.Printf("listen: %v", err)
		return 1
	}
	logger.Printf("api listening on %s", listener.Addr())

	served := make(chan error, 1)
	go func() { served <- srv.Serve(listener) }()

	purgeCtx, stopPurge := context.WithCancel(ctx)
	defer stopPurge()
	purged := make(chan struct{})
	go func() {
		defer close(purged)
		purgeNonces(purgeCtx, st, logger)
	}()

	exit := 0
	select {
	case err := <-served:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Printf("serve: %v", err)
			exit = 1
		}
		_ = srv.Close()
		stopPurge()
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Printf("shutdown: %v", err)
			exit = 1
		}
		cancel()
		stopPurge()
	}
	<-purged
	return exit
}

func newHandler(st *store.Store, cfg config.Config) http.Handler {
	return api.NewMux(st, cfg,
		func(ctx context.Context, meta store.SessionMeta, catalog []byte,
			blobs map[string][]byte) (store.IngestResult, error) {
			return ingest.Run(ctx, st, meta, catalog, blobs)
		},
		func(ctx context.Context, profile *store.Profile) error {
			return ingest.UpdateProfile(ctx, st, profile)
		},
	)
}

// Replay rejection does not depend on this cleanup running.
func purgeNonces(ctx context.Context, st *store.Store, logger *log.Logger) {
	ticker := time.NewTicker(noncePurgeInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			purgeCtx, cancel := context.WithTimeout(ctx, noncePurgeInterval)
			removed, err := st.PurgeNonces(purgeCtx, time.Now())
			cancel()
			if err != nil {
				if ctx.Err() == nil {
					logger.Printf("nonce purge: %v", err)
				}
				continue
			}
			if removed > 0 {
				logger.Printf("nonce purge: removed %d expired rows", removed)
			}
		}
	}
}

func healthcheckExit(logger *log.Logger) int {
	ctx, cancel := context.WithTimeout(context.Background(), healthcheckTimeout)
	defer cancel()
	if err := checkHealth(ctx, probeTarget); err != nil {
		logger.Printf("healthcheck: %v", err)
		return 1
	}
	return 0
}

func checkHealth(ctx context.Context, target string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: healthcheckTimeout}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 64))
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", response.StatusCode)
	}
	if text := string(body); text != "ok" && text != "ok\n" {
		return fmt.Errorf("body is not ok")
	}
	return nil
}
