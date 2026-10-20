package main

import (
	"strings"
	"testing"
)

func testContainerID() string { return strings.Repeat("a", 64) }

func testArgs() []string {
	return []string{
		"-test-only",
		"-source-database-url", "postgres://heliolytics:test@127.0.0.1:5445/heliolytics_rebuild_source_1?sslmode=disable",
		"-target-database-url", "postgres://heliolytics:test@127.0.0.1:5445/heliolytics_rebuild_target?sslmode=disable",
		"-source-test-container-id", testContainerID(),
		"-target-test-container-id", testContainerID(),
	}
}

func TestOptionsValidateRejectsMissingInputs(t *testing.T) {
	complete := options{
		testOnly:        true,
		sourceURL:       "postgres://heliolytics:test@127.0.0.1:5445/heliolytics_rebuild_source_1?sslmode=disable",
		targetURL:       "postgres://heliolytics:test@127.0.0.1:5445/heliolytics_rebuild_target?sslmode=disable",
		sourceContainer: testContainerID(),
		targetContainer: testContainerID(),
	}
	if err := complete.validate(); err != nil {
		t.Fatalf("complete options rejected: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*options)
		want   string
	}{
		{"test-only", func(o *options) { o.testOnly = false }, "-test-only"},
		{"source url", func(o *options) { o.sourceURL = "" }, "-source-database-url"},
		{"target url", func(o *options) { o.targetURL = "" }, "-target-database-url"},
		{"source container", func(o *options) { o.sourceContainer = "" }, "-source-test-container-id"},
		{"target container", func(o *options) { o.targetContainer = "" }, "-target-test-container-id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := complete
			tc.mutate(&opts)
			err := opts.validate()
			if err == nil {
				t.Fatal("incomplete options were accepted")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not name %q", err, tc.want)
			}
		})
	}
}

func TestRunRefusesMissingAcknowledgement(t *testing.T) {
	t.Setenv(imageEnv, "timescale/timescaledb:approved")

	args := testArgs()[1:]
	var stderr strings.Builder
	if code := run(args, &stderr); code == 0 {
		t.Fatal("rebuild ran without -test-only")
	}
	if !strings.Contains(stderr.String(), "-test-only") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunRefusesMissingTestImage(t *testing.T) {
	t.Setenv(imageEnv, "")

	var stderr strings.Builder
	if code := run(testArgs(), &stderr); code == 0 {
		t.Fatal("rebuild ran without an approved test image")
	}
	if !strings.Contains(stderr.String(), imageEnv) {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunRefusesProductionDatabase(t *testing.T) {
	t.Setenv(imageEnv, "timescale/timescaledb:approved")

	args := testArgs()
	args[2] = "postgres://heliolytics:test@127.0.0.1:5445/heliolytics?sslmode=disable"
	var stderr strings.Builder
	if code := run(args, &stderr); code == 0 {
		t.Fatal("rebuild accepted the production database name")
	}
	if !strings.Contains(stderr.String(), "production database") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunRefusesNonLoopbackHost(t *testing.T) {
	t.Setenv(imageEnv, "timescale/timescaledb:approved")

	args := testArgs()
	args[2] = "postgres://heliolytics:test@10.0.0.5:5445/heliolytics_rebuild_source_1?sslmode=disable"
	var stderr strings.Builder
	if code := run(args, &stderr); code == 0 {
		t.Fatal("rebuild accepted a remote host")
	}
	if !strings.Contains(stderr.String(), "loopback") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunRefusesShortContainerID(t *testing.T) {
	t.Setenv(imageEnv, "timescale/timescaledb:approved")

	args := testArgs()
	args[6] = "abc123"
	var stderr strings.Builder
	if code := run(args, &stderr); code == 0 {
		t.Fatal("rebuild accepted a short container ID")
	}
}

func TestParseOptionsRejectsUnknownFlag(t *testing.T) {
	var stderr strings.Builder
	if _, err := parseOptions([]string{"-reset"}, &stderr); err == nil {
		t.Fatal("parseOptions accepted an unknown flag")
	}
}

func TestRunRejectsPositionalArguments(t *testing.T) {
	t.Setenv(imageEnv, "timescale/timescaledb:approved")

	args := append(testArgs(), "extra")
	var stderr strings.Builder
	if code := run(args, &stderr); code == 0 {
		t.Fatal("rebuild accepted a positional argument")
	}
}
