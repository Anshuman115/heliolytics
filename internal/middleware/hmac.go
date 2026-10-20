package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/Anshuman115/heliolytics/internal/auth"
)

type NonceStore interface {
	ConsumeNonce(ctx context.Context, kid, nonce string, expiresAt time.Time) (bool, error)
}

// PrepareFunc runs after the bounded body read and hash comparison but before
// the nonce is consumed. It may parse the body and return a value for the
// handler.
type PrepareFunc func(r *http.Request, body []byte) (any, error)

type GuardConfig struct {
	Keys     auth.KeySet
	Policy   IPPolicy
	IPLimit  *Limiter
	KeyLimit *Limiter
	Slots    *Semaphore
	Nonces   NonceStore
	Clock    func() time.Time
	Logger   *log.Logger
}

type Guard struct {
	keys     auth.KeySet
	policy   IPPolicy
	ipLimit  *Limiter
	keyLimit *Limiter
	slots    *Semaphore
	nonces   NonceStore
	clock    func() time.Time
	logger   *log.Logger
}

func NewGuard(cfg GuardConfig) *Guard {
	if cfg.Nonces == nil {
		panic("middleware.NewGuard: nil nonce store")
	}
	clock := cfg.Clock
	if clock == nil {
		clock = time.Now
	}
	logger := cfg.Logger
	if logger == nil {
		logger = log.Default()
	}
	return &Guard{
		keys:     cfg.Keys,
		policy:   cfg.Policy,
		ipLimit:  cfg.IPLimit,
		keyLimit: cfg.KeyLimit,
		slots:    cfg.Slots,
		nonces:   cfg.Nonces,
		clock:    clock,
		logger:   logger,
	}
}

func (g *Guard) Protect(maxBody int64, prepare PrepareFunc, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		outcome := OutcomeFrom(r)

		header, declaredHash, contentType, reason, failure := cheapHeaders(r)
		if failure != nil {
			outcome.SetReason(reason)
			WriteError(w, failure)
			return
		}

		client, err := g.policy.ClientKey(r)
		if err != nil {
			outcome.SetReason("proxy_header")
			WriteCode(w, http.StatusBadRequest, CodeInvalidProxyHeader)
			return
		}
		outcome.SetClient(client)
		if allowed, retry := g.ipLimit.Allow(client); !allowed {
			outcome.SetReason("rate_limited_ip")
			WriteRetryAfter(w, retry)
			return
		}

		key, err := g.keys.Lookup(header.Kid)
		if err != nil {
			outcome.SetReason("auth_unknown_key")
			WriteCode(w, http.StatusUnauthorized, CodeUnauthorized)
			return
		}
		if !auth.WindowOK(g.clock(), header.Timestamp) {
			outcome.SetReason("auth_time")
			WriteCode(w, http.StatusUnauthorized, CodeUnauthorized)
			return
		}
		if err := key.Verify(header, r.Method, r.URL.EscapedPath(), r.URL.RawQuery, declaredHash, contentType); err != nil {
			outcome.SetReason("auth_signature")
			WriteCode(w, http.StatusUnauthorized, CodeUnauthorized)
			return
		}
		outcome.SetKid(header.Kid)

		if !key.CanWrite() && r.Method != http.MethodGet {
			outcome.SetReason("forbidden_write")
			WriteCode(w, http.StatusForbidden, CodeForbidden)
			return
		}
		if allowed, retry := g.keyLimit.Allow(header.Kid); !allowed {
			outcome.SetReason("rate_limited_key")
			WriteRetryAfter(w, retry)
			return
		}

		if maxBody > 0 {
			if !g.slots.Acquire() {
				outcome.SetReason("body_busy")
				w.Header().Set("Retry-After", "1")
				WriteCode(w, http.StatusServiceUnavailable, CodeServerBusy)
				return
			}
			defer g.slots.Release()
		}

		body, failure := readBody(w, r, maxBody)
		if failure != nil {
			if failure.Status == http.StatusServiceUnavailable {
				outcome.SetReason("request_timeout")
			} else {
				outcome.SetReason("body_invalid")
			}
			WriteError(w, failure)
			return
		}
		if !bodyDigestMatches(body, declaredHash) {
			outcome.SetReason("body_hash")
			WriteCode(w, http.StatusUnauthorized, CodeUnauthorized)
			return
		}

		var value any
		if prepare != nil {
			parsed, err := prepare(r, body)
			if err != nil {
				var controlled *Error
				if errors.As(err, &controlled) {
					outcome.SetReason("body_invalid")
					WriteError(w, controlled)
					return
				}
				g.logger.Printf("route preparation failed method=%s route=%s: %v", r.Method, r.URL.Path, err)
				outcome.SetReason("internal")
				WriteCode(w, http.StatusInternalServerError, CodeInternal)
				return
			}
			value = parsed
		}

		if !auth.WindowOK(g.clock(), header.Timestamp) {
			outcome.SetReason("auth_time")
			WriteCode(w, http.StatusUnauthorized, CodeUnauthorized)
			return
		}

		expiresAt := time.Unix(header.Timestamp, 0).Add(auth.NonceValidity)
		consumed, err := g.nonces.ConsumeNonce(r.Context(), header.Kid, header.Nonce, expiresAt)
		if err != nil {
			g.logger.Printf("nonce store failed kid=%s: %v", header.Kid, err)
			outcome.SetReason("nonce_error")
			WriteCode(w, http.StatusInternalServerError, CodeInternal)
			return
		}
		if !consumed {
			outcome.SetReason("auth_replay")
			WriteCode(w, http.StatusUnauthorized, CodeUnauthorized)
			return
		}

		r = WithKid(r, header.Kid)
		ctx := context.WithValue(r.Context(), bodyKey{}, body)
		if prepare != nil {
			ctx = context.WithValue(ctx, payloadKey{}, value)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func cheapHeaders(r *http.Request) (auth.Header, string, string, string, *Error) {
	values := r.Header.Values(auth.HeaderAuth)
	if len(values) != 1 {
		return auth.Header{}, "", "", "auth_missing", Fail(http.StatusUnauthorized, CodeUnauthorized)
	}
	header, err := auth.ParseHeader(values[0])
	if err != nil {
		return auth.Header{}, "", "", "auth_malformed", Fail(http.StatusUnauthorized, CodeUnauthorized)
	}
	hashes := r.Header.Values(auth.HeaderBodySHA256)
	if len(hashes) != 1 || !auth.IsBodyHash(hashes[0]) {
		return auth.Header{}, "", "", "auth_malformed", Fail(http.StatusUnauthorized, CodeUnauthorized)
	}
	contentType, failure := requestContentType(r)
	if failure != nil {
		return auth.Header{}, "", "", "body_invalid", failure
	}
	return header, hashes[0], contentType, "", nil
}

func requestContentType(r *http.Request) (string, *Error) {
	values := r.Header.Values("Content-Type")
	if len(values) > 1 {
		return "", Fail(http.StatusBadRequest, CodeInvalidBody)
	}
	value := ""
	if len(values) == 1 {
		value = values[0]
	}
	if strings.Contains(value, ",") {
		return "", Fail(http.StatusBadRequest, CodeInvalidBody)
	}
	if encodings := r.Header.Values("Content-Encoding"); len(encodings) > 0 {
		if len(encodings) != 1 || !strings.EqualFold(encodings[0], "identity") {
			return "", Fail(http.StatusBadRequest, CodeInvalidBody)
		}
	}
	switch r.Method {
	case http.MethodGet:
		if value != "" || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
			return "", Fail(http.StatusBadRequest, CodeInvalidBody)
		}
		return "", nil
	case http.MethodPut:
		if value != "application/json" {
			return "", Fail(http.StatusBadRequest, CodeInvalidBody)
		}
		return value, nil
	case http.MethodPost:
		mediaType, params, err := mime.ParseMediaType(value)
		if err != nil || mediaType != "multipart/form-data" {
			return "", Fail(http.StatusBadRequest, CodeInvalidBody)
		}
		if len(params) != 1 || params["boundary"] == "" {
			return "", Fail(http.StatusBadRequest, CodeInvalidBody)
		}
		return value, nil
	default:
		return "", Fail(http.StatusBadRequest, CodeInvalidBody)
	}
}

func readBody(w http.ResponseWriter, r *http.Request, maxBody int64) ([]byte, *Error) {
	if maxBody <= 0 {
		if r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
			return nil, Fail(http.StatusBadRequest, CodeInvalidBody)
		}
		return nil, nil
	}
	if r.ContentLength > maxBody {
		return nil, Fail(http.StatusRequestEntityTooLarge, CodePayloadTooLarge)
	}
	controller := http.NewResponseController(w)
	if deadline, ok := r.Context().Deadline(); ok {
		_ = controller.SetReadDeadline(deadline)
		defer func() { _ = controller.SetReadDeadline(time.Time{}) }()
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	if err != nil {
		if r.Context().Err() != nil {
			return nil, Fail(http.StatusServiceUnavailable, CodeRequestTimeout)
		}
		return nil, Fail(http.StatusBadRequest, CodeInvalidBody)
	}
	if int64(len(data)) > maxBody {
		return nil, Fail(http.StatusRequestEntityTooLarge, CodePayloadTooLarge)
	}
	r.Body = io.NopCloser(bytes.NewReader(data))
	return data, nil
}

func bodyDigestMatches(body []byte, declaredHash string) bool {
	sum := sha256.Sum256(body)
	return subtle.ConstantTimeCompare([]byte(hex.EncodeToString(sum[:])), []byte(declaredHash)) == 1
}

type kidKey struct{}
type bodyKey struct{}
type payloadKey struct{}

func WithKid(r *http.Request, kid string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), kidKey{}, kid))
}

func Kid(r *http.Request) string {
	kid, _ := r.Context().Value(kidKey{}).(string)
	return kid
}

func RequestBody(r *http.Request) []byte {
	body, _ := r.Context().Value(bodyKey{}).([]byte)
	return body
}

func Payload(r *http.Request) any { return r.Context().Value(payloadKey{}) }
