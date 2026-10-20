package parse

import (
	"bytes"
	"encoding/json"
)

// RoundSegment places one device fetch round at a byte offset in the upload.
type RoundSegment struct {
	ByteOffset int    `json:"byteOffset"`
	RoundStart string `json:"roundStart"`
}

// CatalogEntry is one chunked[] entry. Keys the parser does not consume, such
// as status, samples, bytes, file and errorByte, are ignored by encoding/json.
type CatalogEntry struct {
	Code          string         `json:"code"`
	RoundStart    string         `json:"roundStart"`
	RoundSegments []RoundSegment `json:"roundSegments"`
}

// Catalog is the chunked part of the app catalog.
type Catalog struct {
	Chunked []CatalogEntry `json:"chunked"`
}

// ParseCatalog decodes the catalog JSON envelope. Envelope and semantic
// validation of the session belong to the caller.
func ParseCatalog(data []byte) (Catalog, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return Catalog{}, newError(ErrCodeInvalidCatalog, "", 0, "catalog is empty")
	}
	if bytes.Equal(trimmed, []byte("null")) {
		return Catalog{}, newError(ErrCodeInvalidCatalog, "", 0, "catalog must be a JSON object")
	}
	var catalog Catalog
	if err := json.Unmarshal(trimmed, &catalog); err != nil {
		return Catalog{}, newError(ErrCodeInvalidCatalog, "", 0, "malformed catalog JSON: %v", err)
	}
	return catalog, nil
}

// FindEntry returns the catalog entry for a type code, or nil when absent.
func FindEntry(catalog Catalog, code string) *CatalogEntry {
	for i := range catalog.Chunked {
		if catalog.Chunked[i].Code == code {
			return &catalog.Chunked[i]
		}
	}
	return nil
}
