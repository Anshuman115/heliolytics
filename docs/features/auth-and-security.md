# Feature : Auth & security

There is no user login on the API. It's a single-owner system: one shared signing
secret gates data requests. `/health` is public.

## The token

Header `X-Heliolytics-Token`, format `ts.nonce.sig`:

```
ts    = unix seconds
nonce = 16 random bytes, hex
sig   = HMAC-SHA256(secret, "ts:nonce")
```

`internal/auth/signing.go` mints (`SignToken`) and verifies (`VerifyToken` /
`VerifyTokenDetail`). `TokenVerifyResult` carries a `Reason` so failures are
diagnosable in logs without leaking the secret to the client : the HTTP response
stays a bare 401.

### Three implementations, one format

| Repo | File |
|---|---|
| Go server | `internal/auth/signing.go` |
| Flutter app | `lib/services/network/heliolytics_token.dart` |
| Next.js web | `lib/api/signing.ts` |

**Change the format in one and you break the other two.** All three must agree
byte-for-byte on the `"ts:nonce"` string being signed.

## Replay defense

Verification allows at most two seconds of future clock skew. This covers
whole-second client timestamps arriving just before the server's clock reaches the
same second, while larger future offsets are rejected. Tokens more than five
minutes old are still rejected.

Replay defense has two layers:

1. **Time window** : `TokenWindow = 5 * time.Minute`, with the bounded future
   tolerance above. A captured token dies quickly.
2. **Nonce store** : `internal/auth/nonce_store.go` remembers nonces within the
   window and rejects reuse. The window bounds memory: nonces older than it are
   evicted.

The nonce store and rate limiter live in process memory, not a shared database.
They are not a distributed multi-instance replay/limit service.

The server validates the timestamp and signature before recording the nonce. An
invalid signature therefore cannot consume a nonce that a later valid request uses.
After a valid signature is accepted, the nonce is consumed atomically and any
replay is rejected.

Signature comparison uses `crypto/subtle` constant-time equality, never `==`.

## Rate limiting

`internal/middleware/ratelimit.go`, `RATE_LIMIT_PER_MIN` (default 120).

Client identity comes from the remote address, **unless `TRUST_PROXY=true`**, in
which case forwarded headers are honored. Get this wrong and you either rate-limit
everyone as one client (behind a proxy without the flag) or let anyone spoof their
identity via a header (flag on without a trusted proxy in front). Set it only when
a trusted reverse proxy is actually terminating traffic.

## Config

`internal/config/config.go` : env only, no config file:

| Var | Default | Notes |
|---|---|---|
| `ADDR` | `:8080` | Listen address |
| `HELIOLYTICS_SIGNING_SECRET` | *(empty)* | **Required.** Empty secret ⇒ `SignToken` errors and verification fails closed |
| `DATABASE_URL` | *(empty)* | Postgres DSN |
| `RATE_LIMIT_PER_MIN` | `120` | Per-client budget |
| `REPARSE_ENABLED` | `false` | Must be literal `"true"` |
| `REPARSE_SECRET` | *(empty)* | Separate secret for reparse |
| `TRUST_PROXY` | `false` | Honor forwarded client IP |

Secrets are **never** committed : they come from the environment. See
`docs/local/deployment.md`.

## Reparse is separately gated

`/api/v1/reparse` is disabled unless `REPARSE_ENABLED=true`. The code checks
`X-Reparse-Secret` only when `REPARSE_SECRET` is non-empty. Configure an independent
secret before enabling it. The handler replays the latest stored session; it is
not a full-history migration runner. The normal HMAC check still applies.

## Key files

| File | Role |
|---|---|
| `internal/auth/signing.go` | Mint + verify |
| `internal/auth/nonce_store.go` | Replay rejection |
| `internal/middleware/hmac.go` | Auth middleware |
| `internal/middleware/ratelimit.go` | Rate limiter |
| `internal/middleware/logging.go` | Request log |
| `internal/config/config.go` | Env config |
