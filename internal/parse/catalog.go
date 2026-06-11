package parse

import "encoding/json"

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

func ParseCatalog(data []byte) Catalog {
	var c Catalog
	_ = json.Unmarshal(data, &c)
	return c
}

func FindEntry(cat Catalog, code string) *CatalogEntry {
	for i := range cat.Chunked {
		if cat.Chunked[i].Code == code {
			return &cat.Chunked[i]
		}
	}
	return nil
}
