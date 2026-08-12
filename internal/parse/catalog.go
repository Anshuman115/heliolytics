package parse

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

var ErrInvalidCatalog = errors.New("invalid catalog")

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
	if len(data) == 0 {
		return Catalog{}, nil
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return Catalog{}, fmt.Errorf("%w: malformed JSON: empty content", ErrInvalidCatalog)
	}
	var c Catalog
	if err := json.Unmarshal(data, &c); err != nil {
		return Catalog{}, fmt.Errorf("%w: malformed JSON: %v", ErrInvalidCatalog, err)
	}
	if bytes.Equal(trimmed, []byte("null")) {
		return Catalog{}, fmt.Errorf("%w: expected a JSON object", ErrInvalidCatalog)
	}
	return c, nil
}

func FindEntry(cat Catalog, code string) *CatalogEntry {
	for i := range cat.Chunked {
		if cat.Chunked[i].Code == code {
			return &cat.Chunked[i]
		}
	}
	return nil
}
