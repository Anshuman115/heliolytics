// Package auth holds the C5 request signing grammar, key material and the
// fixed kid permissions. Verification here is pure: it never reads the body,
// consults a database or looks at the process clock on its own.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Protocol constants from C5.
const (
	HeaderAuth       = "X-Heliolytics-Auth"
	HeaderBodySHA256 = "X-Heliolytics-Content-SHA256"
	Version          = "v2"
	KidPhone         = "phone"
	KidWeb           = "web"

	// NonceValidity is the offset from the signed timestamp at which a
	// (kid, nonce) pair can no longer authenticate.
	NonceValidity = 301 * time.Second

	pastSkewSeconds   = 300
	futureSkewSeconds = 60
	maxHeaderLength   = 512
)

// Verification failures. Callers answer every one of them with a uniform 401
// and log the error value as a reason code, never the supplied header text.
var (
	ErrMissingHeader   = errors.New("auth: missing credential")
	ErrMalformedHeader = errors.New("auth: malformed credential")
	ErrUnknownKey      = errors.New("auth: unknown key")
	ErrBadSignature    = errors.New("auth: signature mismatch")
)

var (
	kidPattern   = regexp.MustCompile(`^[a-z][a-z0-9-]{1,15}$`)
	noncePattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
	lowerHex     = regexp.MustCompile(`^[0-9a-f]+$`)
)

// Header is a parsed X-Heliolytics-Auth value.
type Header struct {
	Kid       string
	Timestamp int64
	Nonce     string
	Signature []byte
}

// ParseHeader reads the exact v2 grammar
// v2.<kid>.<ts>.<nonce>.<sig>. It rejects every other shape, including the
// removed v1 form.
func ParseHeader(value string) (Header, error) {
	if value == "" {
		return Header{}, ErrMissingHeader
	}
	if len(value) > maxHeaderLength {
		return Header{}, ErrMalformedHeader
	}
	parts := strings.Split(value, ".")
	if len(parts) != 5 || parts[0] != Version {
		return Header{}, ErrMalformedHeader
	}
	if !kidPattern.MatchString(parts[1]) {
		return Header{}, ErrMalformedHeader
	}
	ts, err := parseSeconds(parts[2])
	if err != nil {
		return Header{}, ErrMalformedHeader
	}
	if !noncePattern.MatchString(parts[3]) {
		return Header{}, ErrMalformedHeader
	}
	if len(parts[4]) != hex.EncodedLen(sha256.Size) || !lowerHex.MatchString(parts[4]) {
		return Header{}, ErrMalformedHeader
	}
	signature, err := hex.DecodeString(parts[4])
	if err != nil {
		return Header{}, ErrMalformedHeader
	}
	return Header{Kid: parts[1], Timestamp: ts, Nonce: parts[3], Signature: signature}, nil
}

// parseSeconds reads a non-negative decimal timestamp without sign or
// overflow.
func parseSeconds(value string) (int64, error) {
	if value == "" {
		return 0, ErrMalformedHeader
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, ErrMalformedHeader
		}
	}
	ts, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, ErrMalformedHeader
	}
	return ts, nil
}

// WindowOK reports whether a signed timestamp may authenticate at now. Both
// edges are inclusive.
func WindowOK(now time.Time, ts int64) bool {
	nowSeconds := now.Unix()
	return ts >= nowSeconds-pastSkewSeconds && ts <= nowSeconds+futureSkewSeconds
}

// CanonicalText builds the C5 string to sign: nine newline-joined fields with
// no trailing newline beyond the separator before an empty final field. The
// method is upper-cased here so a caller cannot sign a lower-case verb.
func CanonicalText(kid string, ts int64, nonce, method, escapedPath, rawQuery, declaredHash, contentType string) string {
	return strings.Join([]string{
		Version,
		kid,
		strconv.FormatInt(ts, 10),
		nonce,
		strings.ToUpper(method),
		escapedPath,
		rawQuery,
		declaredHash,
		contentType,
	}, "\n")
}

// IsBodyHash reports whether s is a 64-character lowercase SHA-256 digest.
func IsBodyHash(s string) bool {
	return len(s) == hex.EncodedLen(sha256.Size) && lowerHex.MatchString(s)
}

// Key is one configured credential. The secret is the raw 32 bytes, not the
// 64-character hex text.
type Key struct {
	Kid    string
	secret [32]byte
}

// CanWrite reports the fixed C5 permission: only the phone kid may POST an
// ingest or PUT a profile. Every kid may read protected GET routes.
func (k Key) CanWrite() bool { return k.Kid == KidPhone }

// Verify checks the MAC over the declared body digest and the exact
// Content-Type value. It never looks at the body itself.
func (k Key) Verify(h Header, method, escapedPath, rawQuery, declaredHash, contentType string) error {
	if k.Kid == "" || k.Kid != h.Kid {
		return ErrUnknownKey
	}
	if len(h.Signature) != sha256.Size {
		return ErrMalformedHeader
	}
	canonical := CanonicalText(k.Kid, h.Timestamp, h.Nonce, method, escapedPath, rawQuery, declaredHash, contentType)
	mac := hmac.New(sha256.New, k.secret[:])
	_, _ = mac.Write([]byte(canonical))
	if !hmac.Equal(mac.Sum(nil), h.Signature) {
		return ErrBadSignature
	}
	return nil
}

// SecretHex returns the configured secret as lowercase hex. It exists for
// construction of test requests, not for logging.
func (k Key) SecretHex() string { return hex.EncodeToString(k.secret[:]) }

// KeySet is the fixed phone and web credential pair.
type KeySet struct {
	keys map[string]Key
}

// NewKeySet builds the credential set from raw secret bytes.
func NewKeySet(phone, web [32]byte) KeySet {
	return KeySet{keys: map[string]Key{
		KidPhone: {Kid: KidPhone, secret: phone},
		KidWeb:   {Kid: KidWeb, secret: web},
	}}
}

// Lookup returns the credential for a kid. Kids outside the fixed pair are
// unknown no matter how they are spelled.
func (s KeySet) Lookup(kid string) (Key, error) {
	key, ok := s.keys[kid]
	if !ok {
		return Key{}, ErrUnknownKey
	}
	return key, nil
}

// Kids lists the configured kid names in ascending order.
func (s KeySet) Kids() []string {
	out := make([]string, 0, len(s.keys))
	for kid := range s.keys {
		out = append(out, kid)
	}
	sort.Strings(out)
	return out
}

// Empty reports whether no credential is configured.
func (s KeySet) Empty() bool { return len(s.keys) == 0 }

// burnedSecretDigests are SHA-256 digests of published or development secrets
// that must never authenticate a running server. Only digests are stored.
var burnedSecretDigests = map[string]struct{}{
	// The public C5 fixture secret from internal/auth/testdata/signing-v2.json.
	"630dcd2966c4336691125448bbb25b4ff412a49c732db2c8abc1b8581bd710dd": {},
	// Development defaults from the removed v1 signing secret.
	"b3cf5152a10b7a596ff461bfb6e04203341d08239fc80a56ce135f187ef29e6e": {},
	"93ec5f0a32098f9e89c82e40979731bbe9cf5f391008333cd96ed586187d4d36": {},
}

// ParseKeySet reads HELIOLYTICS_API_KEYS. The grammar is exactly
// web.<64 lowercase hex>,phone.<64 lowercase hex> in either order: both fixed
// kids once each, distinct secrets, no empty entries, no burned secret.
func ParseKeySet(spec string) (KeySet, error) {
	if strings.TrimSpace(spec) == "" {
		return KeySet{}, errors.New("auth: api keys are required")
	}
	secrets := make(map[string][32]byte, 2)
	for _, entry := range strings.Split(spec, ",") {
		if entry == "" {
			return KeySet{}, errors.New("auth: api keys contain an empty entry")
		}
		kid, secretHex, ok := strings.Cut(entry, ".")
		if !ok || !kidPattern.MatchString(kid) {
			return KeySet{}, errors.New("auth: api key entries must be <kid>.<64 lowercase hex>")
		}
		if _, seen := secrets[kid]; seen {
			return KeySet{}, errors.New("auth: api keys contain a duplicate kid")
		}
		if len(secretHex) != hex.EncodedLen(32) || !lowerHex.MatchString(secretHex) {
			return KeySet{}, errors.New("auth: api key secrets must be 64 lowercase hex characters")
		}
		raw, err := hex.DecodeString(secretHex)
		if err != nil {
			return KeySet{}, errors.New("auth: api key secret is not hexadecimal")
		}
		var secret [32]byte
		copy(secret[:], raw)
		digest := sha256.Sum256(secret[:])
		if _, burned := burnedSecretDigests[hex.EncodeToString(digest[:])]; burned {
			return KeySet{}, errors.New("auth: api key secret is a published example")
		}
		secrets[kid] = secret
	}
	phone, hasPhone := secrets[KidPhone]
	web, hasWeb := secrets[KidWeb]
	if !hasPhone || !hasWeb {
		return KeySet{}, errors.New("auth: api keys must contain exactly the phone and web kids")
	}
	if len(secrets) != 2 {
		return KeySet{}, errors.New("auth: api keys must contain exactly the phone and web kids")
	}
	if phone == web {
		return KeySet{}, errors.New("auth: phone and web secrets must differ")
	}
	return KeySet{keys: map[string]Key{
		KidPhone: {Kid: KidPhone, secret: phone},
		KidWeb:   {Kid: KidWeb, secret: web},
	}}, nil
}
