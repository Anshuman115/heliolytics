package auth

import "testing"

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
	if VerifyToken(secret, tok) {
		t.Fatal("replay accepted")
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
