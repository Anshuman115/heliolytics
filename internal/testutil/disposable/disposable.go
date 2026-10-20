// Package disposable verifies local Docker test-database scope and identity.
package disposable

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Database struct {
	ContainerID      string
	Project          string
	Volume           string
	Host             string
	Port             int
	SystemIdentifier string
}

const (
	testLabel          = "heliolytics.test"
	projectLabel       = "heliolytics.test.project"
	dataDestination    = "/var/lib/postgresql/data"
	postgresPort       = "5432/tcp"
	systemIDQuery      = "SELECT system_identifier FROM pg_control_system()"
	productionDatabase = "heliolytics"
	productionVolume   = "deploy_pgdata"
)

var (
	containerIDPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	systemIDPattern    = regexp.MustCompile(`^[0-9]{1,20}$`)
)

func Inspect(ctx context.Context, containerID, dsn, wantImage string) (Database, error) {
	if !containerIDPattern.MatchString(containerID) {
		return Database{}, fmt.Errorf("container ID %q is not a full 64-character lowercase hex ID", containerID)
	}

	target, err := parseDSN(dsn)
	if err != nil {
		return Database{}, err
	}

	info, err := inspectContainer(ctx, containerID)
	if err != nil {
		return Database{}, err
	}

	found, err := validateContainer(containerID, wantImage, target, info)
	if err != nil {
		return Database{}, err
	}

	local, err := localSystemIdentifier(ctx, containerID, target.User, target.Database)
	if err != nil {
		return Database{}, err
	}

	remote, err := remoteSystemIdentifier(ctx, dsn)
	if err != nil {
		return Database{}, err
	}

	identifier, err := matchSystemIdentifier(local, remote)
	if err != nil {
		return Database{}, err
	}

	found.SystemIdentifier = identifier
	return found, nil
}

type dsnInfo struct {
	Host     string
	Port     int
	User     string
	Database string
}

func parseDSN(dsn string) (dsnInfo, error) {
	if strings.TrimSpace(dsn) == "" {
		return dsnInfo{}, errors.New("database URL is empty")
	}

	cfg, err := pgconn.ParseConfig(dsn)
	if err != nil {
		return dsnInfo{}, fmt.Errorf("parse database URL: %w", err)
	}

	for _, fallback := range cfg.Fallbacks {
		if fallback.Host != cfg.Host || fallback.Port != cfg.Port {
			return dsnInfo{}, errors.New("database URL must name exactly one host")
		}
	}
	if cfg.Host != "127.0.0.1" && cfg.Host != "::1" {
		return dsnInfo{}, fmt.Errorf("database URL host %q is not the loopback host 127.0.0.1 or ::1", cfg.Host)
	}
	if cfg.Database == "" {
		return dsnInfo{}, errors.New("database URL must include a database name")
	}
	if cfg.Database == productionDatabase {
		return dsnInfo{}, fmt.Errorf("database %q is the production database", cfg.Database)
	}

	return dsnInfo{
		Host:     cfg.Host,
		Port:     int(cfg.Port),
		User:     cfg.User,
		Database: cfg.Database,
	}, nil
}

type portBinding struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}

type mountInfo struct {
	Type        string `json:"Type"`
	Name        string `json:"Name"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
}

type containerInfo struct {
	ID     string `json:"Id"`
	Config struct {
		Image  string            `json:"Image"`
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	Mounts          []mountInfo `json:"Mounts"`
	NetworkSettings struct {
		Ports map[string][]portBinding `json:"Ports"`
	} `json:"NetworkSettings"`
}

func parseInspectJSON(data []byte) (containerInfo, error) {
	var containers []containerInfo
	if err := json.Unmarshal(data, &containers); err != nil {
		return containerInfo{}, fmt.Errorf("parse docker inspect output: %w", err)
	}
	if len(containers) != 1 {
		return containerInfo{}, fmt.Errorf("docker inspect returned %d containers, want 1", len(containers))
	}
	if containers[0].ID == "" {
		return containerInfo{}, errors.New("docker inspect output has no container ID")
	}
	return containers[0], nil
}

func validateContainer(containerID, wantImage string, target dsnInfo, info containerInfo) (Database, error) {
	if info.ID != containerID {
		return Database{}, fmt.Errorf("docker inspect reported container %s, want %s", info.ID, containerID)
	}

	labels := info.Config.Labels
	if labels[testLabel] != "true" {
		return Database{}, fmt.Errorf("container is missing the %s=true label", testLabel)
	}

	project := strings.TrimSpace(labels[projectLabel])
	if project == "" {
		return Database{}, fmt.Errorf("container is missing the %s label", projectLabel)
	}
	if project == "deploy" {
		return Database{}, fmt.Errorf("%s label %q is the deployment project", projectLabel, project)
	}

	if wantImage == "" {
		return Database{}, errors.New("approved test image is required")
	}
	if info.Config.Image != wantImage {
		return Database{}, fmt.Errorf("container image %q is not the approved test image %q", info.Config.Image, wantImage)
	}

	volume, err := dataVolume(info.Mounts)
	if err != nil {
		return Database{}, err
	}

	if err := validatePortBindings(info.NetworkSettings.Ports[postgresPort], target.Host, target.Port); err != nil {
		return Database{}, err
	}

	return Database{
		ContainerID: info.ID,
		Project:     project,
		Volume:      volume,
		Host:        target.Host,
		Port:        target.Port,
	}, nil
}

func dataVolume(mounts []mountInfo) (string, error) {
	var found *mountInfo
	for i := range mounts {
		if mounts[i].Destination != dataDestination {
			continue
		}
		if found != nil {
			return "", fmt.Errorf("container mounts %s more than once", dataDestination)
		}
		found = &mounts[i]
	}

	if found == nil {
		return "", fmt.Errorf("container has no volume mount at %s", dataDestination)
	}
	if found.Type != "volume" {
		return "", fmt.Errorf("mount at %s is a %q mount, want a Docker volume", dataDestination, found.Type)
	}
	if found.Name == "" {
		return "", fmt.Errorf("volume mounted at %s has no name", dataDestination)
	}
	if found.Name == productionVolume {
		return "", fmt.Errorf("volume %q is the deployment data volume", found.Name)
	}
	return found.Name, nil
}

func validatePortBindings(bindings []portBinding, host string, port int) error {
	switch {
	case len(bindings) == 0:
		return fmt.Errorf("container publishes no loopback port for %s", postgresPort)
	case len(bindings) > 2:
		return fmt.Errorf("container publishes %d ports for %s, want 1 or 2 loopback mappings", len(bindings), postgresPort)
	}

	var ipv4, ipv6 *portBinding
	for i := range bindings {
		switch bindings[i].HostIP {
		case "127.0.0.1":
			if ipv4 != nil {
				return fmt.Errorf("container publishes %s twice on 127.0.0.1", postgresPort)
			}
			ipv4 = &bindings[i]
		case "::1":
			if ipv6 != nil {
				return fmt.Errorf("container publishes %s twice on ::1", postgresPort)
			}
			ipv6 = &bindings[i]
		default:
			return fmt.Errorf("port mapping for %s binds host IP %q, not a loopback address", postgresPort, bindings[i].HostIP)
		}
	}

	if ipv4 == nil {
		return fmt.Errorf("container publishes no %s mapping on 127.0.0.1", postgresPort)
	}
	if host == "::1" && ipv6 == nil {
		return fmt.Errorf("database URL host ::1 has no ::1 port mapping for %s", postgresPort)
	}

	for _, binding := range []*portBinding{ipv4, ipv6} {
		if binding == nil {
			continue
		}
		hostPort, err := strconv.Atoi(binding.HostPort)
		if err != nil {
			return fmt.Errorf("port mapping for %s has host port %q: %w", postgresPort, binding.HostPort, err)
		}
		if hostPort != port {
			return fmt.Errorf("database URL port %d is not the published loopback port %d", port, hostPort)
		}
	}
	return nil
}

func normalizeSystemIdentifier(raw any) (string, error) {
	var text string
	switch value := raw.(type) {
	case int64:
		text = strconv.FormatInt(value, 10)
	case string:
		text = value
	case []byte:
		text = string(value)
	default:
		return "", fmt.Errorf("system identifier has unexpected type %T", raw)
	}

	text = strings.TrimSpace(text)
	if !systemIDPattern.MatchString(text) {
		return "", fmt.Errorf("system identifier %q is not a decimal number", text)
	}
	return text, nil
}

func matchSystemIdentifier(local, remote string) (string, error) {
	local = strings.TrimSpace(local)
	remote = strings.TrimSpace(remote)

	if !systemIDPattern.MatchString(local) {
		return "", fmt.Errorf("container system identifier %q is not a decimal number", local)
	}
	if !systemIDPattern.MatchString(remote) {
		return "", fmt.Errorf("database system identifier %q is not a decimal number", remote)
	}

	localID, err := strconv.ParseUint(local, 10, 64)
	if err != nil {
		return "", fmt.Errorf("container system identifier %q: %w", local, err)
	}
	remoteID, err := strconv.ParseUint(remote, 10, 64)
	if err != nil {
		return "", fmt.Errorf("database system identifier %q: %w", remote, err)
	}
	if localID != remoteID {
		return "", fmt.Errorf("database system identifier %d is not the container cluster %d", remoteID, localID)
	}
	return strconv.FormatUint(localID, 10), nil
}

func inspectContainer(ctx context.Context, containerID string) (containerInfo, error) {
	out, err := runDocker(ctx, "inspect", containerID)
	if err != nil {
		return containerInfo{}, fmt.Errorf("inspect container %s: %w", containerID, err)
	}
	return parseInspectJSON(out)
}

func localSystemIdentifier(ctx context.Context, containerID, user, database string) (string, error) {
	out, err := runDocker(ctx, "exec", containerID, "psql", "-U", user, "-d", database, "-Atc", systemIDQuery)
	if err != nil {
		return "", fmt.Errorf("query system identifier inside container %s: %w", containerID, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func remoteSystemIdentifier(ctx context.Context, dsn string) (string, error) {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return "", fmt.Errorf("connect to database: %w", err)
	}
	defer conn.Close(context.WithoutCancel(ctx))

	var raw any
	if err := conn.QueryRow(ctx, systemIDQuery).Scan(&raw); err != nil {
		return "", fmt.Errorf("query system identifier: %w", err)
	}
	return normalizeSystemIdentifier(raw)
}

func runDocker(ctx context.Context, args ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, "docker", args...).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, err
	}
	return out, nil
}
