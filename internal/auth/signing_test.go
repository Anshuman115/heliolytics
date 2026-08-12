package auth

import (
	"encoding/hex"
	"fmt"
	"testing"
	"time"
)

func TestSignAndVerify(t *testing.T) {
	secret := "test-secret"
	tok, err := SignToken(secret)
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyToken(secret, tok) {
		t.Fatal("valid token rejected")
	}
	if VerifyToken(secret, "bad.token.here") {
		t.Fatal("invalid token accepted")
	}
	if VerifyToken("other", tok) {
		t.Fatal("wrong secret accepted")
	}
}

func TestReplayRejected(t *testing.T) {
	secret := "test-secret"
	tok, err := SignToken(secret)
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyToken(secret, tok) {
		t.Fatal("valid token rejected")
	}
	if result := VerifyTokenDetail(secret, tok); result.OK || result.Reason != "nonce_replay" {
		t.Fatalf("replay result = %+v, want nonce_replay", result)
	}
}

func TestVerifyTokenDetailReasons(t *testing.T) {
	if r := VerifyTokenDetail("", "a.b.c"); r.OK || r.Reason != "server_secret_empty" {
		t.Fatalf("got %+v", r)
	}
	if r := VerifyTokenDetail("s", ""); r.OK || r.Reason != "missing_token_header" {
		t.Fatalf("got %+v", r)
	}
	if r := VerifyTokenDetail("s", "bad"); r.OK || r.Reason != "malformed_token" {
		t.Fatalf("got %+v", r)
	}
	tok, err := SignToken("right-secret")
	if err != nil {
		t.Fatal(err)
	}
	if r := VerifyTokenDetail("wrong-secret", tok); r.OK || r.Reason != "signature_mismatch" {
		t.Fatalf("got %+v", r)
	}
}

func TestVerifyTokenAllowsSmallFutureSkew(t *testing.T) {
	const secret = "test-secret"
	token := tokenAt(time.Now().Unix()+1, secret)
	if r := VerifyTokenDetail(secret, token); !r.OK {
		t.Fatalf("small future skew rejected: %+v", r)
	}
}

func TestVerifyTokenRejectsLargeFutureSkew(t *testing.T) {
	const secret = "test-secret"
	token := tokenAt(time.Now().Add(TokenFutureSkew+5*time.Second).Unix(), secret)
	if r := VerifyTokenDetail(secret, token); r.OK || r.Reason != "token_in_future" {
		t.Fatalf("large future skew accepted: %+v", r)
	}
}

func TestInvalidSignatureDoesNotConsumeNonce(t *testing.T) {
	const (
		secret      = "test-secret"
		wrongSecret = "wrong-secret"
	)
	ts := time.Now().Unix()
	nonce, err := randomHex(16)
	if err != nil {
		t.Fatal(err)
	}

	invalid := tokenWithNonce(ts, nonce, wrongSecret)
	if result := VerifyTokenDetail(secret, invalid); result.OK || result.Reason != "signature_mismatch" {
		t.Fatalf("invalid signature result = %+v, want signature_mismatch", result)
	}

	valid := tokenWithNonce(ts, nonce, secret)
	if result := VerifyTokenDetail(secret, valid); !result.OK {
		t.Fatalf("valid token rejected after invalid signature: %+v", result)
	}
	if result := VerifyTokenDetail(secret, valid); result.OK || result.Reason != "nonce_replay" {
		t.Fatalf("replay result = %+v, want nonce_replay", result)
	}
}

func tokenAt(ts int64, secret string) string {
	nonce, err := randomHex(16)
	if err != nil {
		panic(err)
	}
	return tokenWithNonce(ts, nonce, secret)
}

func tokenWithNonce(ts int64, nonce, secret string) string {
	return fmt.Sprintf("%d.%s.%s", ts, nonce, hex.EncodeToString(sign(secret, ts, nonce)))
}
