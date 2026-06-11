package parse

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestParseReadinessSixByteRecords(t *testing.T) {
	raw := make([]byte, 12)
	sec := int64(1718006400)
	binary.LittleEndian.PutUint32(raw[0:], uint32(sec))
	raw[5] = 78
	binary.LittleEndian.PutUint32(raw[6:], uint32(sec+86400))
	raw[11] = 82
	out := ParseReadiness(raw)
	if len(out) != 2 {
		t.Fatalf("want 2 got %d", len(out))
	}
	if out[0].Readiness != 78 || out[1].Readiness != 82 {
		t.Fatalf("scores %+v", out)
	}
}

func TestParseReadinessHelioStride(t *testing.T) {
	p := filepath.Join("..", "..", "..", "Heliolytics_App", "helio_dump_v5", "0x39_raw.bin")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Skip("dump missing")
	}
	out := ParseReadiness(raw)
	if len(out) < 5 {
		t.Fatalf("want >=5 got %d", len(out))
	}
	if out[0].Readiness != 22 {
		t.Fatalf("score %d", out[0].Readiness)
	}
}
