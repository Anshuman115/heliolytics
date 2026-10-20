package ingest

import (
	"context"
	"errors"
	"testing"

	"github.com/Anshuman115/heliolytics/internal/store"
)

func validScope() RebuildTestScope {
	return RebuildTestScope{
		Source: DisposableDatabase{
			ContainerID: "source-container",
			Project:     "heliolytics-v01",
			Volume:      "heliolytics-v01_pgdata",
			Host:        "127.0.0.1",
			Port:        5445,
			Identity: store.DatabaseIdentity{
				SystemIdentifier: "7361234567890123456",
				DatabaseOID:      16385,
				Database:         "heliolytics_test_0123456789abcdef",
			},
		},
		Target: DisposableDatabase{
			ContainerID: "target-container",
			Project:     "heliolytics-v01",
			Volume:      "heliolytics-v01_target_pgdata",
			Host:        "::1",
			Port:        5446,
			Identity: store.DatabaseIdentity{
				SystemIdentifier: "7361234567890123456",
				DatabaseOID:      16386,
				Database:         "heliolytics_rebuild_target",
			},
		},
	}
}

func TestRebuildScopeRejectsProductionResources(t *testing.T) {
	cases := map[string]func(*RebuildTestScope){
		"missing container": func(s *RebuildTestScope) { s.Target.ContainerID = "" },
		"deploy project":    func(s *RebuildTestScope) { s.Target.Project = "deploy" },
		"empty project":     func(s *RebuildTestScope) { s.Source.Project = "" },
		"deploy volume":     func(s *RebuildTestScope) { s.Source.Volume = "deploy_pgdata" },
		"empty volume":      func(s *RebuildTestScope) { s.Target.Volume = "" },
		"localhost host":    func(s *RebuildTestScope) { s.Source.Host = "localhost" },
		"remote host":       func(s *RebuildTestScope) { s.Target.Host = "10.0.0.5" },
		"zero port":         func(s *RebuildTestScope) { s.Target.Port = 0 },
		"no identity":       func(s *RebuildTestScope) { s.Source.Identity.SystemIdentifier = "" },
		"no oid":            func(s *RebuildTestScope) { s.Target.Identity.DatabaseOID = 0 },
		"production name": func(s *RebuildTestScope) {
			s.Target.Identity.Database = productionDatabase
		},
		"unknown target name": func(s *RebuildTestScope) {
			s.Target.Identity.Database = "postgres"
		},
		"smoke source as target": func(s *RebuildTestScope) {
			s.Target.Identity.Database = smokeSource
		},
	}
	for name, mutate := range cases {
		scope := validScope()
		mutate(&scope)
		err := RebuildAll(context.Background(), nil, nil, scope, nil)
		if err == nil {
			t.Fatalf("%s: rebuild accepted a disallowed scope", name)
		}
		if errors.Is(err, ErrRebuildSameDatabase) {
			t.Fatalf("%s: rebuild reported the wrong refusal", name)
		}
	}
}

func TestRebuildRefusesSameDatabase(t *testing.T) {
	scope := validScope()
	scope.Target.Identity = scope.Source.Identity
	err := RebuildAll(context.Background(), nil, nil, scope, nil)
	if !errors.Is(err, ErrRebuildSameDatabase) {
		t.Fatalf("error = %v, want ErrRebuildSameDatabase", err)
	}
}

func TestRebuildAllowedNames(t *testing.T) {
	allowed := []struct {
		role string
		name string
	}{
		{role: "source", name: "heliolytics_test_0123456789abcdef"},
		{role: "source", name: smokeSource},
		{role: "source", name: "heliolytics_rebuild_source_b012"},
		{role: "target", name: "heliolytics_test_0123456789abcdef"},
		{role: "target", name: "heliolytics_rebuild_target"},
		{role: "target", name: "heliolytics_rebuild_target_b012"},
	}
	for _, tc := range allowed {
		if !allowedDatabaseName(tc.role, tc.name) {
			t.Fatalf("%s %q must be allowed", tc.role, tc.name)
		}
	}
	denied := []struct {
		role string
		name string
	}{
		{role: "source", name: productionDatabase},
		{role: "target", name: productionDatabase},
		{role: "target", name: smokeSource},
		{role: "source", name: "heliolytics_rebuild_target"},
		{role: "target", name: "heliolytics_rebuild_source_b012"},
		{role: "source", name: "heliolytics_test_zzzz"},
	}
	for _, tc := range denied {
		if allowedDatabaseName(tc.role, tc.name) {
			t.Fatalf("%s %q must be denied", tc.role, tc.name)
		}
	}
}
