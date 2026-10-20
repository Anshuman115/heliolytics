package parse

import (
	"errors"
	"testing"
)

func TestParseCatalogRejectsMalformedJSON(t *testing.T) {
	tests := []struct{ name, raw string }{
		{name: "empty", raw: ""},
		{name: "whitespace only", raw: " \n\t"},
		{name: "truncated object", raw: `{"chunked":[`},
		{name: "wrong root type", raw: `[]`},
		{name: "null root", raw: `null`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseCatalog([]byte(test.raw))
			var parseErr *ParseError
			if !errors.As(err, &parseErr) {
				t.Fatalf("error = %v, want a *ParseError", err)
			}
			if parseErr.Code != ErrCodeInvalidCatalog {
				t.Fatalf("code = %s, want %s", parseErr.Code, ErrCodeInvalidCatalog)
			}
		})
	}
}

func TestParseCatalogIgnoresUnknownKeys(t *testing.T) {
	raw := `{"capturedAt":"2026-06-21T11:00:00.000","sessionId":"fixture","unsolicited":[],
		"chunked":[{"code":"0x01","status":"ok","samples":3,"bytes":24,
		"file":"0x01_raw.bin","errorByte":0,
		"roundStart":"2026-06-21T11:16:00.000",
		"roundSegments":[{"byteOffset":0,"roundStart":"2026-06-21T11:16:00.000"}]}]}`
	catalog, err := ParseCatalog([]byte(raw))
	if err != nil {
		t.Fatalf("parse catalog: %v", err)
	}
	entry := FindEntry(catalog, typeActivity)
	if entry == nil {
		t.Fatal("0x01 entry is missing")
	}
	if entry.RoundStart != "2026-06-21T11:16:00.000" {
		t.Errorf("round start = %q", entry.RoundStart)
	}
	if len(entry.RoundSegments) != 1 || entry.RoundSegments[0].ByteOffset != 0 {
		t.Errorf("round segments = %+v", entry.RoundSegments)
	}
}

func TestFindEntryReturnsNilForAbsentCode(t *testing.T) {
	catalog, err := ParseCatalog([]byte(`{"chunked":[{"code":"0x01"}]}`))
	if err != nil {
		t.Fatalf("parse catalog: %v", err)
	}
	if entry := FindEntry(catalog, typeHRV); entry != nil {
		t.Fatalf("found %+v for an absent code", entry)
	}
	if entry := FindEntry(catalog, typeActivity); entry == nil {
		t.Fatal("0x01 entry is missing")
	}
}
