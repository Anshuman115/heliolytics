package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"mime"
	"mime/multipart"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Anshuman115/heliolytics/internal/middleware"
	"github.com/Anshuman115/heliolytics/internal/parse"
	"github.com/Anshuman115/heliolytics/internal/store"
)

const maxIngestParts = 40

var ingestTypeCodes = map[string]struct{}{
	"0x01": {}, "0x05": {}, "0x06": {}, "0x0D": {}, "0x13": {}, "0x25": {},
	"0x26": {}, "0x2E": {}, "0x38": {}, "0x46": {}, "0x48": {}, "0x49": {},
}

var macPattern = regexp.MustCompile(`^[0-9A-F]{2}(:[0-9A-F]{2}){5}$`)

type ingestHandler struct {
	run IngestFunc
}

type ingestPayload struct {
	session sessionFields
	catalog []byte
	blobs   map[string][]byte
}

type sessionFields struct {
	sessionID string
	startedAt string
	endedAt   string
	deviceMAC string
	battery   *json.Number
}

// Reject structural envelope errors before consuming the nonce.
func (h *ingestHandler) prepare(r *http.Request, body []byte) (any, error) {
	_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return nil, middleware.Fail(http.StatusBadRequest, middleware.CodeInvalidBody)
	}
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])

	var (
		sessionPart []byte
		catalogPart []byte
		haveSession bool
		haveCatalog bool
		parts       int
	)
	blobs := make(map[string][]byte, len(ingestTypeCodes))
	for {
		part, err := reader.NextRawPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, middleware.Fail(http.StatusBadRequest, middleware.CodeInvalidBody)
		}
		parts++
		if parts > maxIngestParts {
			return nil, middleware.Fail(http.StatusBadRequest, middleware.CodeInvalidBody)
		}
		if encoding := part.Header.Get("Content-Transfer-Encoding"); encoding != "" &&
			!strings.EqualFold(encoding, "identity") {
			return nil, middleware.Fail(http.StatusBadRequest, middleware.CodeInvalidBody)
		}
		if mediaType := part.Header.Get("Content-Type"); strings.HasPrefix(
			strings.ToLower(strings.TrimSpace(mediaType)), "multipart/") {
			return nil, middleware.Fail(http.StatusBadRequest, middleware.CodeInvalidBody)
		}
		name := part.FormName()
		if name == "" {
			return nil, middleware.Fail(http.StatusBadRequest, middleware.CodeInvalidBody)
		}
		data, err := io.ReadAll(part)
		if err != nil {
			return nil, middleware.Fail(http.StatusBadRequest, middleware.CodeInvalidBody)
		}
		switch name {
		case "session":
			if haveSession {
				return nil, middleware.Fail(http.StatusBadRequest, middleware.CodeInvalidBody)
			}
			haveSession = true
			sessionPart = data
		case "catalog":
			if haveCatalog {
				return nil, middleware.Fail(http.StatusBadRequest, middleware.CodeInvalidBody)
			}
			haveCatalog = true
			catalogPart = data
		default:
			code, ok := rawTypeCode(name)
			if !ok {
				return nil, middleware.Fail(http.StatusBadRequest, middleware.CodeUnknownPart)
			}
			if _, duplicate := blobs[code]; duplicate {
				return nil, middleware.Fail(http.StatusBadRequest, middleware.CodeInvalidBody)
			}
			blobs[code] = data
		}
	}
	if !haveSession || !haveCatalog {
		return nil, middleware.Fail(http.StatusBadRequest, middleware.CodeInvalidBody)
	}
	if !hasNonEmptyBlob(blobs) {
		return nil, middleware.Fail(http.StatusBadRequest, middleware.CodeEmptyUpload)
	}

	fields, failure := strictObject(sessionPart)
	if failure != nil {
		return nil, failure
	}
	session, failure := sessionFieldsOf(fields)
	if failure != nil {
		return nil, failure
	}
	return &ingestPayload{session: session, catalog: catalogPart, blobs: blobs}, nil
}

func (h *ingestHandler) serve(w http.ResponseWriter, r *http.Request) {
	payload, ok := middleware.Payload(r).(*ingestPayload)
	if !ok {
		middleware.WriteCode(w, http.StatusInternalServerError, middleware.CodeInternal)
		return
	}
	meta, failure := payload.session.meta()
	if failure != nil {
		middleware.WriteError(w, failure)
		return
	}
	if failure := crossCheckCatalog(payload.catalog, meta.ID); failure != nil {
		middleware.WriteError(w, failure)
		return
	}
	result, err := h.run(r.Context(), meta, payload.catalog, payload.blobs)
	if err != nil {
		writeIngestError(w, err)
		return
	}
	normalizeIngestResult(&result)
	writeJSON(w, http.StatusOK, result)
}

type catalogEnvelope struct {
	SessionID string `json:"sessionId"`
	Chunked   []struct {
		Code string `json:"code"`
	} `json:"chunked"`
}

func crossCheckCatalog(catalog []byte, sessionID string) *middleware.Error {
	var envelope catalogEnvelope
	if err := json.Unmarshal(catalog, &envelope); err != nil {
		return nil
	}
	if envelope.SessionID != "" && envelope.SessionID != sessionID {
		return middleware.Fail(http.StatusUnprocessableEntity, middleware.CodeInvalidCatalog)
	}
	seen := make(map[string]struct{}, len(envelope.Chunked))
	for _, entry := range envelope.Chunked {
		if entry.Code == "" {
			continue
		}
		if _, duplicate := seen[entry.Code]; duplicate {
			return middleware.Fail(http.StatusUnprocessableEntity, middleware.CodeInvalidCatalog)
		}
		seen[entry.Code] = struct{}{}
	}
	return nil
}

func (f sessionFields) meta() (store.SessionMeta, *middleware.Error) {
	invalid := middleware.Fail(http.StatusUnprocessableEntity, middleware.CodeInvalidSession)
	if f.sessionID == "" || len(f.sessionID) > 64 || !printableASCII(f.sessionID) {
		return store.SessionMeta{}, invalid
	}
	startedAt, err := parseInstant(f.startedAt)
	if err != nil {
		return store.SessionMeta{}, invalid
	}
	endedAt, err := parseInstant(f.endedAt)
	if err != nil {
		return store.SessionMeta{}, invalid
	}
	if endedAt.Before(startedAt) {
		return store.SessionMeta{}, invalid
	}
	if !macPattern.MatchString(f.deviceMAC) {
		return store.SessionMeta{}, invalid
	}
	var battery *int
	if f.battery != nil {
		value, err := strconv.ParseFloat(string(*f.battery), 64)
		if err != nil || math.Trunc(value) != value || value < 0 || value > 100 || math.IsInf(value, 0) {
			return store.SessionMeta{}, invalid
		}
		percent := int(value)
		battery = &percent
	}
	return store.SessionMeta{
		ID:         f.sessionID,
		DeviceMAC:  f.deviceMAC,
		StartedAt:  startedAt,
		EndedAt:    endedAt,
		BatteryPct: battery,
	}, nil
}

func parseInstant(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed, nil
}

func printableASCII(value string) bool {
	for _, r := range value {
		if r < 0x20 || r > 0x7e {
			return false
		}
	}
	return true
}

// rawTypeCode maps a part name to an accepted band type code. Identity comes
// from the part name, never from the filename.
func rawTypeCode(name string) (string, bool) {
	code, ok := strings.CutSuffix(name, "_raw.bin")
	if !ok || len(code) != 4 || !strings.HasPrefix(code, "0x") {
		return "", false
	}
	if _, accepted := ingestTypeCodes[code]; !accepted {
		return "", false
	}
	return code, true
}

func hasNonEmptyBlob(blobs map[string][]byte) bool {
	for _, data := range blobs {
		if len(data) > 0 {
			return true
		}
	}
	return false
}

func sessionFieldsOf(fields map[string]json.RawMessage) (sessionFields, *middleware.Error) {
	invalid := middleware.Fail(http.StatusBadRequest, middleware.CodeInvalidBody)
	var out sessionFields
	targets := map[string]*string{
		"sessionId": &out.sessionID,
		"startedAt": &out.startedAt,
		"endedAt":   &out.endedAt,
		"deviceMac": &out.deviceMAC,
	}
	for name, target := range targets {
		raw, ok := fields[name]
		if !ok {
			return sessionFields{}, invalid
		}
		value, ok := decodeString(raw)
		if !ok {
			return sessionFields{}, invalid
		}
		*target = value
	}
	if raw, ok := fields["batteryPercent"]; ok {
		number, ok := decodeNullableNumber(raw)
		if !ok {
			return sessionFields{}, invalid
		}
		out.battery = number
	}
	for name := range fields {
		if _, required := targets[name]; required || name == "batteryPercent" {
			continue
		}
		return sessionFields{}, invalid
	}
	return out, nil
}

func writeIngestError(w http.ResponseWriter, err error) {
	var parseErr *parse.ParseError
	var invalidRow *store.InvalidRowError
	switch {
	case errors.As(err, &parseErr):
		middleware.WriteError(w, parseFailure(parseErr))
	case errors.Is(err, store.ErrSessionConflict):
		middleware.WriteCode(w, http.StatusConflict, middleware.CodeSessionConflict)
	case errors.As(err, &invalidRow):
		middleware.WriteCode(w, http.StatusUnprocessableEntity, middleware.CodeInvalidSession)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		middleware.WriteCode(w, http.StatusServiceUnavailable, middleware.CodeRequestTimeout)
	default:
		log.Printf("ingest failed: %v", err)
		middleware.WriteCode(w, http.StatusInternalServerError, middleware.CodeInternal)
	}
}

func parseFailure(parseErr *parse.ParseError) *middleware.Error {
	code := middleware.CodeInvalidType
	switch parseErr.Code {
	case parse.ErrCodeInvalidCatalog, parse.ErrCodeMissingAnchor:
		code = middleware.CodeInvalidCatalog
	case parse.ErrCodeInvalidWindow:
		code = middleware.CodeInvalidSession
	}
	failure := middleware.Fail(http.StatusUnprocessableEntity, code)
	if parseErr.TypeCode != "" {
		failure.Message = failure.Error() + " for band type " + parseErr.TypeCode
	}
	return failure
}

func normalizeIngestResult(result *store.IngestResult) {
	if result.Types == nil {
		result.Types = make(map[string]store.TypeResult)
	}
	for code, value := range result.Types {
		if value.Warnings == nil {
			value.Warnings = []string{}
			result.Types[code] = value
		}
	}
	if result.Warnings == nil {
		result.Warnings = []string{}
	}
}

func strictObject(body []byte) (map[string]json.RawMessage, *middleware.Error) {
	invalid := middleware.Fail(http.StatusBadRequest, middleware.CodeInvalidBody)
	decoder := json.NewDecoder(bytes.NewReader(body))
	token, err := decoder.Token()
	if err != nil {
		return nil, invalid
	}
	if open, ok := token.(json.Delim); !ok || open != '{' {
		return nil, invalid
	}
	out := make(map[string]json.RawMessage)
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, invalid
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, invalid
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, invalid
		}
		if _, duplicate := out[key]; duplicate {
			return nil, invalid
		}
		out[key] = raw
	}
	if _, err := decoder.Token(); err != nil {
		return nil, invalid
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, invalid
	}
	return out, nil
}

func decodeString(raw json.RawMessage) (string, bool) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	return value, true
}

func decodeNullableString(raw json.RawMessage) (*string, bool) {
	trimmed := bytes.TrimSpace(raw)
	if string(trimmed) == "null" {
		return nil, true
	}
	value, ok := decodeString(trimmed)
	if !ok {
		return nil, false
	}
	return &value, true
}

func decodeNullableNumber(raw json.RawMessage) (*json.Number, bool) {
	trimmed := bytes.TrimSpace(raw)
	if string(trimmed) == "null" {
		return nil, true
	}
	if len(trimmed) == 0 {
		return nil, false
	}
	if first := trimmed[0]; (first < '0' || first > '9') && first != '-' {
		return nil, false
	}
	var number json.Number
	if err := json.Unmarshal(trimmed, &number); err != nil {
		return nil, false
	}
	return &number, true
}
