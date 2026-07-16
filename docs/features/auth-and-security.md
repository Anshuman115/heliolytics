# Feature — Auth & security

There is no user login on the API. It's a single-owner system: one shared signing
secret gates every request.

## The token

Header `X-Heliolytics-Token`, format `ts.nonce.sig`:

```
ts    = unix seconds
nonce = 16 random bytes, hex
sig   = HMAC-SHA256(secret, "ts:nonce")
```

`internal/auth/signing.go` mints (`SignToken`) and verifies (`VerifyToken` /
`VerifyTokenDetail`). `TokenVerifyResult` carries a `Reason` so failures are
diagnosable in logs without leaking the secret to the client — the HTTP response
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

Two layers:

1. **Time window** — `TokenWindow = 5 * time.Minute`. Tokens outside it are rejected,
   so a captured token dies quickly.
2. **Nonce store** — `internal/auth/nonce_store.go` remembers nonces within the
   window and rejects reuse. The window bounds memory: nonces older than it are
   evicted.

Comparison uses `crypto/subtle` constant-time equality — never `==` on signatures.

## Rate limiting

`internal/middleware/ratelimit.go`, `RATE_LIMIT_PER_MIN` (default 120).

Client identity comes from the remote address, **unless `TRUST_PROXY=true`**, in
which case forwarded headers are honored. Get this wrong and you either rate-limit
everyone as one client (behind a proxy without the flag) or let anyone spoof their
identity via a header (flag on without a trusted proxy in front). Set it only when
Caddy is actually terminating.

## Config

`internal/config/config.go` — env only, no config file:

| Var | Default | Notes |
|---|---|---|
| `ADDR` | `:8080` | Listen address |
| `HELIOLYTICS_SIGNING_SECRET` | *(empty)* | **Required.** Empty secret ⇒ `SignToken` errors and verification fails closed |
| `DATABASE_URL` | *(empty)* | Postgres DSN |
| `RATE_LIMIT_PER_MIN` | `120` | Per-client budget |
| `REPARSE_ENABLED` | `false` | Must be literal `"true"` |
| `REPARSE_SECRET` | *(empty)* | Separate secret for reparse |
| `TRUST_PROXY` | `false` | Honor forwarded client IP |

Secrets are **never** committed — they come from the environment. See
`deploy/deployment.md`.

## Reparse is separately gated

`/api/v1/reparse` needs both `REPARSE_ENABLED=true` and its own `REPARSE_SECRET`,
distinct from the main signing secret. It rewrites historical data, so it's off in
production and enabled only for a deliberate migration.

## Key files

| File | Role |
|---|---|
| `internal/auth/signing.go` | Mint + verify |
| `internal/auth/nonce_store.go` | Replay rejection |
| `internal/middleware/hmac.go` | Auth middleware |
| `internal/middleware/ratelimit.go` | Rate limiter |
| `internal/middleware/logging.go` | Request log |
| `internal/config/config.go` | Env config |
