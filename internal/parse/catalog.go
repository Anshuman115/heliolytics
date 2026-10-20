package parse

import (
	"bytes"
	"encoding/json"
)

type RoundSegment struct {
	ByteOffset int    `json:"byteOffset"`
	RoundStart string `json:"roundStart"`
}

type CatalogEntry struct {
	Code          string         `json:"code"`
	RoundStart    string         `json:"roundStart"`
	RoundSegments []RoundSegment `json:"roundSegments"`
}

type Catalog struct {
	Chunked []CatalogEntry `json:"chunked"`
}

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

func FindEntry(catalog Catalog, code string) *CatalogEntry {
	for i := range catalog.Chunked {
		if catalog.Chunked[i].Code == code {
			return &catalog.Chunked[i]
		}
	}
	return nil
}
