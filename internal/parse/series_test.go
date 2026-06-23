package parse

import (
	"encoding/binary"
	"testing"
)

func TestParseRespRateSeries(t *testing.T) {
	raw := make([]byte, 8)
	sec := int64(1700000000)
	binary.LittleEndian.PutUint32(raw[0:], uint32(sec))
	raw[5] = 14
	out := ParseRespRateSeries(raw)
	if len(out) != 1 || out[0].Metric != "resp_rate" || int(out[0].Value) != 14 {
		t.Fatalf("got %+v", out)
	}
}
