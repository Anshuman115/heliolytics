package disposable

import (
	"context"
	"strings"
	"testing"
)

const (
	testContainerID = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	testImage       = "timescale/timescaledb:2.30.2-pg16@sha256:fc16b2459b5faf9f84c010c189cfd77267bbadf1514c3bfd20215eebf41fe5e4"
	testLabels      = `"heliolytics.test":"true","heliolytics.test.project":"v01"`
	testMounts      = `{"Type":"volume","Name":"test-vol","Source":"/var/lib/docker/volumes/test-vol/_data","Destination":"/var/lib/postgresql/data"}`
	testPorts       = `"5432/tcp":[{"HostIp":"127.0.0.1","HostPort":"5445"}]`
)

// testInspectJSON renders the docker inspect shape from literal fragments.
func testInspectJSON(id, image, labels, mounts, ports string) string {
	return `[{"Id":"` + id + `","Config":{"Image":"` + image + `","Labels":{` + labels + `}},` +
		`"Mounts":[` + mounts + `],"NetworkSettings":{"Ports":{` + ports + `}}}]`
}

func validTarget() dsnInfo {
	return dsnInfo{Host: "127.0.0.1", Port: 5445, User: "heliolytics", Database: "heliolytics_test_admin"}
}

func TestParseDSN(t *testing.T) {
	tests := []struct {
		name      string
		dsn       string
		want      dsnInfo
		wantError string
	}{
		{
			name: "ipv4 url",
			dsn:  "postgres://heliolytics:secret@127.0.0.1:5445/heliolytics_test_admin?sslmode=disable",
			want: dsnInfo{Host: "127.0.0.1", Port: 5445, User: "heliolytics", Database: "heliolytics_test_admin"},
		},
		{
			name: "ipv6 url",
			dsn:  "postgres://heliolytics:secret@[::1]:5445/heliolytics_test_admin?sslmode=disable",
			want: dsnInfo{Host: "::1", Port: 5445, User: "heliolytics", Database: "heliolytics_test_admin"},
		},
		{
			name: "keyword form",
			dsn:  "host=127.0.0.1 port=5445 user=heliolytics password=secret dbname=heliolytics_test_admin sslmode=disable",
			want: dsnInfo{Host: "127.0.0.1", Port: 5445, User: "heliolytics", Database: "heliolytics_test_admin"},
		},
		{
			name:      "empty",
			dsn:       "  ",
			wantError: "database URL is empty",
		},
		{
			name:      "unparseable",
			dsn:       "not a dsn",
			wantError: "parse database URL",
		},
		{
			name:      "localhost host",
			dsn:       "postgres://heliolytics:secret@localhost:5445/heliolytics_test_admin?sslmode=disable",
			wantError: "not the loopback host",
		},
		{
			name:      "wildcard host",
			dsn:       "postgres://heliolytics:secret@0.0.0.0:5445/heliolytics_test_admin?sslmode=disable",
			wantError: "not the loopback host",
		},
		{
			name:      "multiple hosts",
			dsn:       "postgres://heliolytics:secret@127.0.0.1:5445,127.0.0.1:5446/heliolytics_test_admin?sslmode=disable",
			wantError: "exactly one host",
		},
		{
			name:      "production database",
			dsn:       "postgres://heliolytics:secret@127.0.0.1:5445/heliolytics?sslmode=disable",
			wantError: "production database",
		},
		{
			name:      "missing database",
			dsn:       "postgres://heliolytics:secret@127.0.0.1:5445/?sslmode=disable",
			wantError: "must include a database name",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseDSN(tc.dsn)
			if tc.wantError != "" {
				if err == nil {
					t.Fatalf("parseDSN(%q) = %+v, want error containing %q", tc.dsn, got, tc.wantError)
				}
				if !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("parseDSN(%q) error = %v, want containing %q", tc.dsn, err, tc.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDSN(%q): %v", tc.dsn, err)
			}
			if got != tc.want {
				t.Fatalf("parseDSN(%q) = %+v, want %+v", tc.dsn, got, tc.want)
			}
		})
	}
}

func TestValidateContainer(t *testing.T) {
	tests := []struct {
		name        string
		containerID string
		wantImage   string
		target      dsnInfo
		json        string
		want        Database
		wantError   string
	}{
		{
			name:        "valid",
			containerID: testContainerID,
			wantImage:   testImage,
			target:      validTarget(),
			json:        testInspectJSON(testContainerID, testImage, testLabels, testMounts, testPorts),
			want: Database{
				ContainerID: testContainerID,
				Project:     "v01",
				Volume:      "test-vol",
				Host:        "127.0.0.1",
				Port:        5445,
			},
		},
		{
			name:        "valid ipv6 alias",
			containerID: testContainerID,
			wantImage:   testImage,
			target:      dsnInfo{Host: "::1", Port: 5445, User: "heliolytics", Database: "heliolytics_test_admin"},
			json: testInspectJSON(testContainerID, testImage, testLabels, testMounts,
				`"5432/tcp":[{"HostIp":"::1","HostPort":"5445"},{"HostIp":"127.0.0.1","HostPort":"5445"}]`),
			want: Database{
				ContainerID: testContainerID,
				Project:     "v01",
				Volume:      "test-vol",
				Host:        "::1",
				Port:        5445,
			},
		},
		{
			name:        "missing test label",
			containerID: testContainerID,
			wantImage:   testImage,
			target:      validTarget(),
			json:        testInspectJSON(testContainerID, testImage, `"heliolytics.test.project":"v01"`, testMounts, testPorts),
			wantError:   "heliolytics.test=true",
		},
		{
			name:        "missing project label",
			containerID: testContainerID,
			wantImage:   testImage,
			target:      validTarget(),
			json:        testInspectJSON(testContainerID, testImage, `"heliolytics.test":"true"`, testMounts, testPorts),
			wantError:   "heliolytics.test.project",
		},
		{
			name:        "deploy project",
			containerID: testContainerID,
			wantImage:   testImage,
			target:      validTarget(),
			json: testInspectJSON(testContainerID, testImage,
				`"heliolytics.test":"true","heliolytics.test.project":"deploy"`, testMounts, testPorts),
			wantError: "deployment project",
		},
		{
			name:        "compose project is ignored",
			containerID: testContainerID,
			wantImage:   testImage,
			target:      validTarget(),
			json: testInspectJSON(testContainerID, testImage,
				`"heliolytics.test":"true","com.docker.compose.project":"v01"`, testMounts, testPorts),
			wantError: "heliolytics.test.project",
		},
		{
			name:        "production volume",
			containerID: testContainerID,
			wantImage:   testImage,
			target:      validTarget(),
			json: testInspectJSON(testContainerID, testImage, testLabels,
				`{"Type":"volume","Name":"deploy_pgdata","Destination":"/var/lib/postgresql/data"}`, testPorts),
			wantError: "deployment data volume",
		},
		{
			name:        "missing data mount",
			containerID: testContainerID,
			wantImage:   testImage,
			target:      validTarget(),
			json: testInspectJSON(testContainerID, testImage, testLabels,
				`{"Type":"volume","Name":"test-vol","Destination":"/var/lib/postgresql/other"}`, testPorts),
			wantError: "no volume mount",
		},
		{
			name:        "bind mount",
			containerID: testContainerID,
			wantImage:   testImage,
			target:      validTarget(),
			json: testInspectJSON(testContainerID, testImage, testLabels,
				`{"Type":"bind","Source":"/tmp/pgdata","Destination":"/var/lib/postgresql/data"}`, testPorts),
			wantError: "want a Docker volume",
		},
		{
			name:        "unnamed volume",
			containerID: testContainerID,
			wantImage:   testImage,
			target:      validTarget(),
			json: testInspectJSON(testContainerID, testImage, testLabels,
				`{"Type":"volume","Destination":"/var/lib/postgresql/data"}`, testPorts),
			wantError: "has no name",
		},
		{
			name:        "container id mismatch",
			containerID: "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			wantImage:   testImage,
			target:      validTarget(),
			json:        testInspectJSON(testContainerID, testImage, testLabels, testMounts, testPorts),
			wantError:   "docker inspect reported container",
		},
		{
			name:        "image mismatch",
			containerID: testContainerID,
			wantImage:   testImage,
			target:      validTarget(),
			json:        testInspectJSON(testContainerID, "timescale/timescaledb:2.30.2-pg16", testLabels, testMounts, testPorts),
			wantError:   "approved test image",
		},
		{
			name:        "missing approved image",
			containerID: testContainerID,
			wantImage:   "",
			target:      validTarget(),
			json:        testInspectJSON(testContainerID, testImage, testLabels, testMounts, testPorts),
			wantError:   "approved test image is required",
		},
		{
			name:        "all interfaces mapping",
			containerID: testContainerID,
			wantImage:   testImage,
			target:      validTarget(),
			json: testInspectJSON(testContainerID, testImage, testLabels, testMounts,
				`"5432/tcp":[{"HostIp":"0.0.0.0","HostPort":"5445"}]`),
			wantError: "not a loopback address",
		},
		{
			name:        "wrong port",
			containerID: testContainerID,
			wantImage:   testImage,
			target:      dsnInfo{Host: "127.0.0.1", Port: 5446, User: "heliolytics", Database: "heliolytics_test_admin"},
			json:        testInspectJSON(testContainerID, testImage, testLabels, testMounts, testPorts),
			wantError:   "is not the published loopback port",
		},
		{
			name:        "no published mapping",
			containerID: testContainerID,
			wantImage:   testImage,
			target:      validTarget(),
			json:        testInspectJSON(testContainerID, testImage, testLabels, testMounts, ``),
			wantError:   "publishes no loopback port",
		},
		{
			name:        "ipv6 alias with another port",
			containerID: testContainerID,
			wantImage:   testImage,
			target:      validTarget(),
			json: testInspectJSON(testContainerID, testImage, testLabels, testMounts,
				`"5432/tcp":[{"HostIp":"127.0.0.1","HostPort":"5445"},{"HostIp":"::1","HostPort":"5447"}]`),
			wantError: "5447",
		},
		{
			name:        "ipv6 host without ipv6 mapping",
			containerID: testContainerID,
			wantImage:   testImage,
			target:      dsnInfo{Host: "::1", Port: 5445, User: "heliolytics", Database: "heliolytics_test_admin"},
			json:        testInspectJSON(testContainerID, testImage, testLabels, testMounts, testPorts),
			wantError:   "has no ::1 port mapping",
		},
		{
			name:        "duplicate ipv4 mapping",
			containerID: testContainerID,
			wantImage:   testImage,
			target:      validTarget(),
			json: testInspectJSON(testContainerID, testImage, testLabels, testMounts,
				`"5432/tcp":[{"HostIp":"127.0.0.1","HostPort":"5445"},{"HostIp":"127.0.0.1","HostPort":"5445"}]`),
			wantError: "twice on 127.0.0.1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			info, err := parseInspectJSON([]byte(tc.json))
			if err != nil {
				t.Fatalf("parseInspectJSON: %v", err)
			}
			got, err := validateContainer(tc.containerID, tc.wantImage, tc.target, info)
			if tc.wantError != "" {
				if err == nil {
					t.Fatalf("validateContainer = %+v, want error containing %q", got, tc.wantError)
				}
				if !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("validateContainer error = %v, want containing %q", err, tc.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateContainer: %v", err)
			}
			if got != tc.want {
				t.Fatalf("validateContainer = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestParseInspectJSON(t *testing.T) {
	tests := []struct {
		name      string
		json      string
		wantError string
	}{
		{
			name: "one container",
			json: testInspectJSON(testContainerID, testImage, testLabels, testMounts, testPorts),
		},
		{
			name:      "empty list",
			json:      `[]`,
			wantError: "want 1",
		},
		{
			name:      "two containers",
			json:      `[{"Id":"a"},{"Id":"b"}]`,
			wantError: "want 1",
		},
		{
			name:      "invalid json",
			json:      `{`,
			wantError: "parse docker inspect output",
		},
		{
			name:      "missing id",
			json:      `[{"Config":{"Image":"x"}}]`,
			wantError: "no container ID",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseInspectJSON([]byte(tc.json))
			if tc.wantError != "" {
				if err == nil {
					t.Fatalf("parseInspectJSON = %+v, want error containing %q", got, tc.wantError)
				}
				if !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("parseInspectJSON error = %v, want containing %q", err, tc.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseInspectJSON: %v", err)
			}
			if got.ID != testContainerID {
				t.Fatalf("parseInspectJSON ID = %q, want %q", got.ID, testContainerID)
			}
		})
	}
}

func TestMatchSystemIdentifier(t *testing.T) {
	tests := []struct {
		name      string
		local     string
		remote    string
		want      string
		wantError string
	}{
		{name: "equal", local: "7452063901203456789", remote: "7452063901203456789", want: "7452063901203456789"},
		{name: "equal with padding", local: " 123 ", remote: "0123", want: "123"},
		{name: "mismatch", local: "123", remote: "456", wantError: "is not the container cluster"},
		{name: "empty remote", local: "123", remote: "", wantError: "not a decimal number"},
		{name: "text local", local: "abc", remote: "abc", wantError: "not a decimal number"},
		{name: "overflow", local: "99999999999999999999", remote: "1", wantError: "value out of range"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := matchSystemIdentifier(tc.local, tc.remote)
			if tc.wantError != "" {
				if err == nil {
					t.Fatalf("matchSystemIdentifier = %q, want error containing %q", got, tc.wantError)
				}
				if !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("matchSystemIdentifier error = %v, want containing %q", err, tc.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("matchSystemIdentifier: %v", err)
			}
			if got != tc.want {
				t.Fatalf("matchSystemIdentifier = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNormalizeSystemIdentifier(t *testing.T) {
	tests := []struct {
		name      string
		raw       any
		want      string
		wantError string
	}{
		{name: "int64", raw: int64(7452063901203456789), want: "7452063901203456789"},
		{name: "string", raw: "123", want: "123"},
		{name: "bytes", raw: []byte("456"), want: "456"},
		{name: "float", raw: 1.5, wantError: "unexpected type"},
		{name: "empty", raw: "", wantError: "not a decimal number"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeSystemIdentifier(tc.raw)
			if tc.wantError != "" {
				if err == nil {
					t.Fatalf("normalizeSystemIdentifier(%v) = %q, want error containing %q", tc.raw, got, tc.wantError)
				}
				if !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("normalizeSystemIdentifier(%v) error = %v, want containing %q", tc.raw, err, tc.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeSystemIdentifier(%v): %v", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("normalizeSystemIdentifier(%v) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestInspectRejectsBeforeInspection(t *testing.T) {
	tests := []struct {
		name        string
		containerID string
		dsn         string
		wantImage   string
		wantError   string
	}{
		{
			name:        "short container id",
			containerID: "heliolytics-v01",
			dsn:         "postgres://heliolytics:secret@127.0.0.1:5445/heliolytics_test_admin?sslmode=disable",
			wantImage:   testImage,
			wantError:   "not a full 64-character lowercase hex ID",
		},
		{
			name:        "empty dsn",
			containerID: testContainerID,
			dsn:         "  ",
			wantImage:   testImage,
			wantError:   "database URL is empty",
		},
		{
			name:        "non loopback host",
			containerID: testContainerID,
			dsn:         "postgres://heliolytics:secret@localhost:5445/heliolytics_test_admin?sslmode=disable",
			wantImage:   testImage,
			wantError:   "not the loopback host",
		},
		{
			name:        "production database",
			containerID: testContainerID,
			dsn:         "postgres://heliolytics:secret@127.0.0.1:5445/heliolytics?sslmode=disable",
			wantImage:   testImage,
			wantError:   "production database",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Inspect(context.Background(), tc.containerID, tc.dsn, tc.wantImage)
			if err == nil {
				t.Fatalf("Inspect = %+v, want error containing %q", got, tc.wantError)
			}
			if !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("Inspect error = %v, want containing %q", err, tc.wantError)
			}
			if got != (Database{}) {
				t.Fatalf("Inspect returned %+v with an error, want the zero value", got)
			}
		})
	}
}
