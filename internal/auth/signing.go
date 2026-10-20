// Package auth implements request signing and credential permissions.
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

const (
	HeaderAuth       = "X-Heliolytics-Auth"
	HeaderBodySHA256 = "X-Heliolytics-Content-SHA256"
	KidPhone         = "phone"
	KidWeb           = "web"

	// NonceValidity is the offset from the signed timestamp at which a
	// (kid, nonce) pair can no longer authenticate.
	NonceValidity = 301 * time.Second

	pastSkewSeconds   = 300
	futureSkewSeconds = 60
	maxHeaderLength   = 512
)

// Return uniform 401 responses; never log supplied credentials.
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

type Header struct {
	Kid       string
	Timestamp int64
	Nonce     string
	Signature []byte
}

func ParseHeader(value string) (Header, error) {
	if value == "" {
		return Header{}, ErrMissingHeader
	}
	if len(value) > maxHeaderLength {
		return Header{}, ErrMalformedHeader
	}
	parts := strings.Split(value, ".")
	if len(parts) != 4 {
		return Header{}, ErrMalformedHeader
	}
	if !kidPattern.MatchString(parts[0]) {
		return Header{}, ErrMalformedHeader
	}
	ts, err := parseSeconds(parts[1])
	if err != nil {
		return Header{}, ErrMalformedHeader
	}
	if !noncePattern.MatchString(parts[2]) {
		return Header{}, ErrMalformedHeader
	}
	if len(parts[3]) != hex.EncodedLen(sha256.Size) || !lowerHex.MatchString(parts[3]) {
		return Header{}, ErrMalformedHeader
	}
	signature, err := hex.DecodeString(parts[3])
	if err != nil {
		return Header{}, ErrMalformedHeader
	}
	return Header{Kid: parts[0], Timestamp: ts, Nonce: parts[2], Signature: signature}, nil
}

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

func WindowOK(now time.Time, ts int64) bool {
	nowSeconds := now.Unix()
	return ts >= nowSeconds-pastSkewSeconds && ts <= nowSeconds+futureSkewSeconds
}

// Join the eight fields without a terminator; an empty Content-Type leaves its separator.
func CanonicalText(kid string, ts int64, nonce, method, escapedPath, rawQuery, declaredHash, contentType string) string {
	return strings.Join([]string{
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

func IsBodyHash(s string) bool {
	return len(s) == hex.EncodedLen(sha256.Size) && lowerHex.MatchString(s)
}

type Key struct {
	Kid    string
	secret [32]byte
}

func (k Key) CanWrite() bool { return k.Kid == KidPhone }

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

// Never log the returned secret.
func (k Key) SecretHex() string { return hex.EncodeToString(k.secret[:]) }

type KeySet struct {
	keys map[string]Key
}

func NewKeySet(phone, web [32]byte) KeySet {
	return KeySet{keys: map[string]Key{
		KidPhone: {Kid: KidPhone, secret: phone},
		KidWeb:   {Kid: KidWeb, secret: web},
	}}
}

func (s KeySet) Lookup(kid string) (Key, error) {
	key, ok := s.keys[kid]
	if !ok {
		return Key{}, ErrUnknownKey
	}
	return key, nil
}

func (s KeySet) Kids() []string {
	out := make([]string, 0, len(s.keys))
	for kid := range s.keys {
		out = append(out, kid)
	}
	sort.Strings(out)
	return out
}

func (s KeySet) Empty() bool { return len(s.keys) == 0 }

// Published signing secrets must never authenticate a running server.
var burnedSecretDigests = map[string]struct{}{
	"630dcd2966c4336691125448bbb25b4ff412a49c732db2c8abc1b8581bd710dd": {},
}

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
