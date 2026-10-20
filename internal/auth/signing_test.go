package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

type signingVector struct {
	ID            string `json:"id"`
	Kid           string `json:"kid"`
	SecretHex     string `json:"secretHex"`
	TS            int64  `json:"ts"`
	Nonce         string `json:"nonce"`
	Method        string `json:"method"`
	EscapedPath   string `json:"escapedPath"`
	RawQuery      string `json:"rawQuery"`
	ContentType   string `json:"contentType"`
	BodyBase64    string `json:"bodyBase64"`
	CanonicalText string `json:"canonicalText"`
	BodySHA256    string `json:"bodySha256"`
	Signature     string `json:"signature"`
	AuthHeader    string `json:"authHeader"`
}

func loadVectors(t *testing.T) []signingVector {
	t.Helper()
	raw, err := os.ReadFile("testdata/signing.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fixture struct {
		Vectors []signingVector `json:"vectors"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if len(fixture.Vectors) != 5 {
		t.Fatalf("vectors = %d, want 5", len(fixture.Vectors))
	}
	return fixture.Vectors
}

func keyFor(t *testing.T, secretHex string) KeySet {
	t.Helper()
	raw, err := hex.DecodeString(secretHex)
	if err != nil || len(raw) != 32 {
		t.Fatalf("secret hex: %v", err)
	}
	var secret [32]byte
	copy(secret[:], raw)
	return NewKeySet(secret, secret)
}

func TestSigningVectorsMatchFixture(t *testing.T) {
	for _, vector := range loadVectors(t) {
		t.Run(vector.ID, func(t *testing.T) {
			body, err := base64.StdEncoding.DecodeString(vector.BodyBase64)
			if err != nil {
				t.Fatalf("body base64: %v", err)
			}
			sum := sha256.Sum256(body)
			if got := hex.EncodeToString(sum[:]); got != vector.BodySHA256 {
				t.Fatalf("body digest = %s, want %s", got, vector.BodySHA256)
			}
			if got := CanonicalText(vector.Kid, vector.TS, vector.Nonce, vector.Method,
				vector.EscapedPath, vector.RawQuery, vector.BodySHA256, vector.ContentType); got != vector.CanonicalText {
				t.Fatalf("canonical text mismatch:\ngot  %q\nwant %q", got, vector.CanonicalText)
			}

			header, err := ParseHeader(vector.AuthHeader)
			if err != nil {
				t.Fatalf("ParseHeader: %v", err)
			}
			if header.Kid != vector.Kid || header.Timestamp != vector.TS || header.Nonce != vector.Nonce {
				t.Fatalf("parsed header = %+v", header)
			}
			if got := hex.EncodeToString(header.Signature); got != vector.Signature {
				t.Fatalf("signature = %s, want %s", got, vector.Signature)
			}

			keys := keyFor(t, vector.SecretHex)
			key, err := keys.Lookup(vector.Kid)
			if err != nil {
				t.Fatalf("Lookup: %v", err)
			}
			if err := key.Verify(header, vector.Method, vector.EscapedPath, vector.RawQuery,
				vector.BodySHA256, vector.ContentType); err != nil {
				t.Fatalf("Verify: %v", err)
			}

			if err := key.Verify(header, vector.Method, vector.EscapedPath+"x", vector.RawQuery,
				vector.BodySHA256, vector.ContentType); err == nil {
				t.Fatal("changed path accepted")
			}
			if err := key.Verify(header, vector.Method, vector.EscapedPath, vector.RawQuery+"&extra=1",
				vector.BodySHA256, vector.ContentType); err == nil {
				t.Fatal("changed query accepted")
			}
			if err := key.Verify(header, vector.Method, vector.EscapedPath, vector.RawQuery,
				vector.BodySHA256, vector.ContentType+" "); err == nil {
				t.Fatal("changed content type accepted")
			}
		})
	}
}

func TestParseHeaderRejectsOtherShapes(t *testing.T) {
	cases := map[string]string{
		"empty":              "",
		"missing kid":        "1760000000.00112233445566778899aabbccddeeff.deadbeef",
		"missing signature":  "phone.1760000000.00112233445566778899aabbccddeeff",
		"unknown prefix":     "apikey.phone.1760000000.00112233445566778899aabbccddeeff." + strings.Repeat("a", 64),
		"bad kid":            "Phone.1760000000.00112233445566778899aabbccddeeff." + strings.Repeat("a", 64),
		"signed timestamp":   "phone.-1760000000.00112233445566778899aabbccddeeff." + strings.Repeat("a", 64),
		"overflow timestamp": "phone.99999999999999999999999.00112233445566778899aabbccddeeff." + strings.Repeat("a", 64),
		"short nonce":        "phone.1760000000.abcd." + strings.Repeat("a", 64),
		"uppercase nonce":    "phone.1760000000.00112233445566778899AABBCCDDEEFF." + strings.Repeat("a", 64),
		"uppercase signature": "phone.1760000000.00112233445566778899aabbccddeeff." +
			strings.Repeat("A", 64),
		"short signature": "phone.1760000000.00112233445566778899aabbccddeeff." + strings.Repeat("a", 32),
		"long header":     "phone.1760000000.00112233445566778899aabbccddeeff." + strings.Repeat("a", 600),
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseHeader(value); err == nil {
				t.Fatal("malformed header accepted")
			}
		})
	}
}

func TestWindowOKEdges(t *testing.T) {
	now := time.Unix(1760000000, 0)
	cases := []struct {
		offset int64
		want   bool
	}{
		{-301, false},
		{-300, true},
		{0, true},
		{60, true},
		{61, false},
	}
	for _, tc := range cases {
		if got := WindowOK(now, now.Unix()+tc.offset); got != tc.want {
			t.Fatalf("offset %d accepted = %v, want %v", tc.offset, got, tc.want)
		}
	}
}

func TestParseKeySet(t *testing.T) {
	first := strings.Repeat("11", 32)
	second := strings.Repeat("22", 32)
	third := strings.Repeat("33", 32)

	valid, err := ParseKeySet("phone." + first + ",web." + second)
	if err != nil {
		t.Fatalf("valid spec rejected: %v", err)
	}
	if kids := valid.Kids(); len(kids) != 2 || kids[0] != KidPhone || kids[1] != KidWeb {
		t.Fatalf("kids = %v", kids)
	}
	if _, err := ParseKeySet("web." + second + ",phone." + first); err != nil {
		t.Fatalf("reordered spec rejected: %v", err)
	}

	cases := map[string]string{
		"empty":             "",
		"empty entry":       "phone." + first + ",",
		"missing web":       "phone." + first,
		"third kid":         "phone." + first + ",web." + second + ",tablet." + third,
		"duplicate kid":     "phone." + first + ",phone." + second,
		"same secret":       "phone." + first + ",web." + first,
		"no dot":            "phone" + first + ",web." + second,
		"short secret":      "phone." + first[:62] + ",web." + second,
		"uppercase hex":     "phone." + strings.Repeat("AA", 32) + ",web." + second,
		"non hex secret":    "phone." + strings.Repeat("zz", 32) + ",web." + second,
		"uppercase kid":     "PHONE." + first + ",web." + second,
		"published example": "phone.000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f,web." + second,
	}
	for name, spec := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseKeySet(spec); err == nil {
				t.Fatal("invalid key spec accepted")
			}
		})
	}
}

func TestKeyPermission(t *testing.T) {
	keys := NewKeySet([32]byte{1}, [32]byte{2})
	phone, err := keys.Lookup(KidPhone)
	if err != nil {
		t.Fatalf("phone lookup: %v", err)
	}
	if !phone.CanWrite() {
		t.Fatal("phone must be allowed to write")
	}
	web, err := keys.Lookup(KidWeb)
	if err != nil {
		t.Fatalf("web lookup: %v", err)
	}
	if web.CanWrite() {
		t.Fatal("web must not be allowed to write")
	}
	if _, err := keys.Lookup("admin"); !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("unknown kid error = %v", err)
	}
}

func TestIsBodyHash(t *testing.T) {
	if !IsBodyHash(strings.Repeat("ab", 32)) {
		t.Fatal("valid digest rejected")
	}
	for _, value := range []string{"", strings.Repeat("ab", 31), strings.Repeat("AB", 32), "zz"} {
		if IsBodyHash(value) {
			t.Fatalf("%q accepted as a digest", value)
		}
	}
}
