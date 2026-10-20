package parse

import (
	"math"
	"testing"
)

func TestReadUvarint(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		want    uint64
		wantErr bool
	}{
		{name: "zero", input: []byte{0x00}, want: 0},
		{name: "single byte maximum", input: []byte{0x7f}, want: 127},
		{name: "two bytes", input: []byte{0x80, 0x01}, want: 128},
		{name: "maximum uint64", input: []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01}, want: math.MaxUint64},
		{name: "tenth byte overflow", input: []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x02}, wantErr: true},
		{name: "tenth byte continues", input: []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80}, wantErr: true},
		{name: "truncated continuation", input: []byte{0x80}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, next, err := readUvarint(test.input, 0)
			if test.wantErr {
				if err == nil {
					t.Fatalf("value=%d next=%d, want an error", got, next)
				}
				return
			}
			if err != nil {
				t.Fatalf("value=%d: %v", got, err)
			}
			if got != test.want || next != len(test.input) {
				t.Fatalf("value=%d next=%d, want %d and %d", got, next, test.want, len(test.input))
			}
		})
	}
}

func TestReadLengthDelimitedChecksRemainingBytes(t *testing.T) {
	if _, next, err := readLengthDelimited([]byte{0x03, 0x61, 0x62, 0x63}, 0); err != nil || next != 4 {
		t.Fatalf("next=%d error=%v, want the three byte payload", next, err)
	}
	if _, _, err := readLengthDelimited([]byte{0x05, 0x61}, 0); err == nil {
		t.Fatal("a length beyond the buffer must fail")
	}
	if _, _, err := readLengthDelimited([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01}, 0); err == nil {
		t.Fatal("an oversized length must fail")
	}
}

func TestReadMessageBoundsNesting(t *testing.T) {
	payload := protoVarintField(nil, 1, 1)
	for range 12 {
		payload = protoBytesField(nil, 2, payload)
	}
	budget := &protoBudget{}
	message, err := readMessage(payload, 0, budget)
	if err != nil {
		t.Fatalf("top message: %v", err)
	}
	for depth := 0; depth < 20; depth++ {
		nested, subErr := subMessage(message, 2, depth, budget)
		if subErr != nil {
			return
		}
		if nested == nil {
			t.Fatal("nesting must be rejected before the payload ends")
		}
		message = nested
	}
	t.Fatal("nesting was not bounded")
}

func TestReadMessageBoundsNodeCount(t *testing.T) {
	payload := make([]byte, 0, (protoMaxNodes+1)*2)
	for range protoMaxNodes + 1 {
		payload = append(payload, 0x08, 0x01)
	}
	if _, err := readMessage(payload, 0, &protoBudget{}); err == nil {
		t.Fatal("an oversized message must fail")
	}
}

func TestReadMessageRejectsZeroTagWithoutPadding(t *testing.T) {
	if _, err := readMessage([]byte{0x08, 0x01, 0x00, 0x01}, 0, &protoBudget{}); err == nil {
		t.Fatal("a zero tag followed by real bytes must fail")
	}
	if _, err := readMessage([]byte{0x08, 0x01, 0x00, 0x80}, 0, &protoBudget{}); err != nil {
		t.Fatalf("trailing record padding must be tolerated: %v", err)
	}
}

func TestWorkoutDecoderRejectsCraftedLengths(t *testing.T) {
	inputs := [][]byte{
		{0x00, 0x80, 0x0a, 0x03, 0x32, 0x2e, 0x31, 0x12, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01, 0x00},
		{0x12, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f, 0x00},
	}
	for i, input := range inputs {
		if _, _, err := parseWorkoutSummaries(input, catalogWindow()); err == nil {
			t.Fatalf("input %d decoded without an error", i)
		}
	}
}
