package store

import (
	"encoding/json"
	"time"
)

func validateSessionMeta(m SessionMeta) error {
	if len(m.ID) == 0 || len(m.ID) > 64 {
		return invalidRow("sync_sessions", "session_id must be 1 to 64 characters")
	}
	for i := range m.ID {
		if m.ID[i] < 0x20 || m.ID[i] > 0x7e {
			return invalidRow("sync_sessions", "session_id must be printable ASCII")
		}
	}
	if m.DeviceMAC == "" {
		return invalidRow("sync_sessions", "device_mac required")
	}
	if m.StartedAt.IsZero() {
		return invalidRow("sync_sessions", "started_at required")
	}
	if m.EndedAt.IsZero() {
		return invalidRow("sync_sessions", "ended_at required")
	}
	if m.EndedAt.Before(m.StartedAt) {
		return invalidRow("sync_sessions", "ended_at before started_at")
	}
	if m.BatteryPct != nil && (*m.BatteryPct < 0 || *m.BatteryPct > 100) {
		return invalidRow("sync_sessions", "battery_pct out of range")
	}
	return nil
}

func validateTypeCode(code string) error {
	if len(code) != 4 || code[0] != '0' || code[1] != 'x' {
		return invalidRow("raw_blobs", "type_code %q invalid", code)
	}
	for i := 2; i < 4; i++ {
		c := code[i]
		if (c >= '0' && c <= '9') || (c >= 'A' && c <= 'F') {
			continue
		}
		return invalidRow("raw_blobs", "type_code %q invalid", code)
	}
	return nil
}

func validateSessionRef(sessionID, table string) error {
	if sessionID == "" {
		return invalidRow(table, "session_id required")
	}
	return nil
}

func validateObservedAt(observedAt time.Time, table string) error {
	if observedAt.IsZero() {
		return invalidRow(table, "observed_at required")
	}
	return nil
}

func validateInstant(at time.Time, table string) error {
	if at.IsZero() {
		return invalidRow(table, "sampled_at required")
	}
	return nil
}

// canonicalJSON normalizes a catalog so property order or number spelling
// alone is not a conflict.
func canonicalJSON(raw []byte) ([]byte, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return json.Marshal(value)
}
