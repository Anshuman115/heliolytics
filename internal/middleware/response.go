package middleware

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// C3.1 error codes.
const (
	CodeInvalidDate        = "invalid_date"
	CodeInvalidRange       = "invalid_range"
	CodeRangeTooLarge      = "range_too_large"
	CodeInvalidBody        = "invalid_body"
	CodeUnknownPart        = "unknown_part"
	CodeEmptyUpload        = "empty_upload"
	CodeInvalidProxyHeader = "invalid_proxy_header"
	CodeInvalidSession     = "invalid_session"
	CodeInvalidCatalog     = "invalid_catalog"
	CodeInvalidType        = "invalid_type"
	CodeInvalidProfile     = "invalid_profile"
	CodeSessionConflict    = "session_conflict"
	CodePayloadTooLarge    = "payload_too_large"
	CodeUnauthorized       = "unauthorized"
	CodeForbidden          = "forbidden"
	CodeRateLimited        = "rate_limited"
	CodeNotFound           = "not_found"
	CodeMethodNotAllowed   = "method_not_allowed"
	CodeServerBusy         = "server_busy"
	CodeRequestTimeout     = "request_timeout"
	CodeInternal           = "internal"
)

// errorMessages holds the only client-visible text for each code. Nothing
// derived from the request, the database or a panic belongs here.
var errorMessages = map[string]string{
	CodeInvalidDate:        "from and to must be YYYY-MM-DD and must appear together",
	CodeInvalidRange:       "from must not be after to",
	CodeRangeTooLarge:      "the requested range is too large",
	CodeInvalidBody:        "the request body is not valid",
	CodeUnknownPart:        "an unknown part was submitted",
	CodeEmptyUpload:        "no non-empty band data part was submitted",
	CodeInvalidProxyHeader: "the proxy header is not valid",
	CodeInvalidSession:     "the session metadata is not valid",
	CodeInvalidCatalog:     "the catalog is not valid",
	CodeInvalidType:        "a submitted band type is not valid",
	CodeInvalidProfile:     "the profile is not valid",
	CodeSessionConflict:    "the session was already stored with different content",
	CodePayloadTooLarge:    "the request body is too large",
	CodeUnauthorized:       "authentication failed",
	CodeForbidden:          "this credential cannot write",
	CodeRateLimited:        "too many requests",
	CodeNotFound:           "no such route",
	CodeMethodNotAllowed:   "method not allowed for this route",
	CodeServerBusy:         "the server is busy",
	CodeRequestTimeout:     "the request timed out",
	CodeInternal:           "internal error",
}

// Error is a C3 error a handler can return with an explicit status and code.
type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return errorMessages[e.Code]
}

// Fail builds an Error with the code's default safe message.
func Fail(status int, code string) *Error { return &Error{Status: status, Code: code} }

// errorEnvelope is the C3.1 error shape.
type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WriteCode writes the JSON error envelope for a known code.
func WriteCode(w http.ResponseWriter, status int, code string) {
	WriteError(w, &Error{Status: status, Code: code})
}

// WriteError writes the JSON error envelope for an Error, defaulting an
// unknown status or code rather than emitting an empty body.
func WriteError(w http.ResponseWriter, e *Error) {
	status := e.Status
	if status == 0 {
		status = http.StatusInternalServerError
	}
	message := e.Message
	if message == "" {
		message = errorMessages[e.Code]
	}
	if message == "" {
		message = errorMessages[CodeInternal]
	}
	body, err := json.Marshal(errorEnvelope{Error: errorBody{Code: e.Code, Message: message}})
	if err != nil {
		http.Error(w, errorMessages[CodeInternal], http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(append(body, '\n'))
}

// WriteRetryAfter writes a 429 with the C3 body and a nonzero Retry-After.
func WriteRetryAfter(w http.ResponseWriter, seconds int) {
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	WriteCode(w, http.StatusTooManyRequests, CodeRateLimited)
}

// HeaderWritten reports whether a known response wrapper has committed a
// response. A writer that cannot answer reports false; the production chain
// always ends in the access logger's status writer, which answers exactly.
func HeaderWritten(w http.ResponseWriter) bool {
	for {
		if state, ok := w.(interface{ HeaderWritten() bool }); ok {
			return state.HeaderWritten()
		}
		unwrapper, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return false
		}
		w = unwrapper.Unwrap()
	}
}
