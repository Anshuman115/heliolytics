package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const TokenWindow = 5 * time.Minute

func SignToken(secret string) (string, error) {
	if secret == "" {
		return "", fmt.Errorf("signing secret not configured")
	}
	nonce, err := randomHex(16)
	if err != nil {
		return "", err
	}
	ts := time.Now().Unix()
	sig := sign(secret, ts, nonce)
	return fmt.Sprintf("%d.%s.%s", ts, nonce, hex.EncodeToString(sig)), nil
}

// TokenVerifyResult explains why HMAC auth failed (Reason empty when OK).
type TokenVerifyResult struct {
	OK     bool
	Reason string
	Detail string
}

func VerifyToken(secret, token string) bool {
	return VerifyTokenDetail(secret, token).OK
}

func VerifyTokenDetail(secret, token string) TokenVerifyResult {
	if secret == "" {
		return TokenVerifyResult{Reason: "server_secret_empty"}
	}
	if token == "" {
		return TokenVerifyResult{Reason: "missing_token_header"}
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return TokenVerifyResult{
			Reason: "malformed_token",
			Detail: fmt.Sprintf("expected 3 dot-separated parts, got %d", len(parts)),
		}
	}
	ts, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return TokenVerifyResult{Reason: "bad_timestamp", Detail: parts[0]}
	}
	tokenTime := time.Unix(ts, 0)
	age := time.Since(tokenTime)
	if age < 0 {
		return TokenVerifyResult{
			Reason: "token_in_future",
			Detail: fmt.Sprintf("skew=%s", (-age).Round(time.Second)),
		}
	}
	if age > TokenWindow {
		return TokenVerifyResult{
			Reason: "token_expired",
			Detail: fmt.Sprintf("age=%s window=%s", age.Round(time.Second), TokenWindow),
		}
	}
	if !defaultNonceStore.UseOnce(parts[1], tokenTime) {
		return TokenVerifyResult{Reason: "nonce_replay"}
	}
	got, err := hex.DecodeString(parts[2])
	if err != nil {
		return TokenVerifyResult{Reason: "bad_signature_hex", Detail: err.Error()}
	}
	want := sign(secret, ts, parts[1])
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return TokenVerifyResult{
			Reason: "signature_mismatch",
			Detail: "HELIOLYTICS_SIGNING_SECRET must match the app API key",
		}
	}
	return TokenVerifyResult{OK: true}
}

func sign(secret string, ts int64, nonce string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(fmt.Sprintf("%d:%s", ts, nonce)))
	return mac.Sum(nil)
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
