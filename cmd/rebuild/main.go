// Command rebuild replays one disposable test database into a different empty target.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Anshuman115/heliolytics/internal/ingest"
	"github.com/Anshuman115/heliolytics/internal/store"
	"github.com/Anshuman115/heliolytics/internal/testutil/disposable"
)

const (
	rebuildTimeout = 30 * time.Minute
	imageEnv       = "HELIOLYTICS_TEST_DB_IMAGE"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

type options struct {
	testOnly        bool
	sourceURL       string
	targetURL       string
	sourceContainer string
	targetContainer string
}

func parseOptions(args []string, stderr io.Writer) (options, error) {
	flags := flag.NewFlagSet("rebuild", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var opts options
	flags.BoolVar(&opts.testOnly, "test-only", false,
		"acknowledge that both databases are disposable local test resources")
	flags.StringVar(&opts.sourceURL, "source-database-url", "", "read-only disposable source database URL")
	flags.StringVar(&opts.targetURL, "target-database-url", "", "empty disposable target database URL")
	flags.StringVar(&opts.sourceContainer, "source-test-container-id", "", "source test database container ID")
	flags.StringVar(&opts.targetContainer, "target-test-container-id", "", "target test database container ID")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	if flags.NArg() > 0 {
		return options{}, fmt.Errorf("unexpected positional arguments: %d", flags.NArg())
	}
	return opts, nil
}

func (o options) validate() error {
	if !o.testOnly {
		return errors.New("-test-only is required; only disposable test databases may be rebuilt")
	}
	if o.sourceURL == "" {
		return errors.New("-source-database-url is required")
	}
	if o.targetURL == "" {
		return errors.New("-target-database-url is required")
	}
	if o.sourceContainer == "" {
		return errors.New("-source-test-container-id is required")
	}
	if o.targetContainer == "" {
		return errors.New("-target-test-container-id is required")
	}
	return nil
}

func run(args []string, stderr io.Writer) int {
	logger := log.New(stderr, "", 0)

	opts, err := parseOptions(args, stderr)
	if err != nil {
		return 2
	}
	if err := opts.validate(); err != nil {
		logger.Printf("rebuild: %v", err)
		return 2
	}
	wantImage := strings.TrimSpace(os.Getenv(imageEnv))
	if wantImage == "" {
		logger.Printf("rebuild: %s is required", imageEnv)
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), rebuildTimeout)
	defer cancel()
	if err := rebuild(ctx, opts, wantImage, logger); err != nil {
		logger.Printf("rebuild: %v", err)
		return 1
	}
	logger.Print("rebuild: success")
	return 0
}

func rebuild(ctx context.Context, opts options, wantImage string, logger *log.Logger) error {
	source, err := disposable.Inspect(ctx, opts.sourceContainer, opts.sourceURL, wantImage)
	if err != nil {
		return fmt.Errorf("source scope: %w", err)
	}
	target, err := disposable.Inspect(ctx, opts.targetContainer, opts.targetURL, wantImage)
	if err != nil {
		return fmt.Errorf("target scope: %w", err)
	}

	sourceStore, err := store.New(ctx, opts.sourceURL)
	if err != nil {
		return fmt.Errorf("source store: %w", err)
	}
	defer sourceStore.Close()
	targetStore, err := store.New(ctx, opts.targetURL)
	if err != nil {
		return fmt.Errorf("target store: %w", err)
	}
	defer targetStore.Close()

	sourceIdentity, err := sourceStore.Identity(ctx)
	if err != nil {
		return fmt.Errorf("source identity: %w", err)
	}
	targetIdentity, err := targetStore.Identity(ctx)
	if err != nil {
		return fmt.Errorf("target identity: %w", err)
	}
	if sourceIdentity == targetIdentity {
		return ingest.ErrRebuildSameDatabase
	}

	scope := ingest.RebuildTestScope{
		Source: disposableDatabase(source, sourceIdentity),
		Target: disposableDatabase(target, targetIdentity),
	}
	return ingest.RebuildAll(ctx, sourceStore, targetStore, scope, func(message string) {
		logger.Print(message)
	})
}

func disposableDatabase(db disposable.Database, identity store.DatabaseIdentity) ingest.DisposableDatabase {
	return ingest.DisposableDatabase{
		ContainerID: db.ContainerID,
		Project:     db.Project,
		Volume:      db.Volume,
		Host:        db.Host,
		Port:        uint16(db.Port),
		Identity:    identity,
	}
}
