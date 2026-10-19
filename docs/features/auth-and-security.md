# Authentication and security

The API has no user login. It serves one owner and gates data requests with one shared signing secret. `/health` is public.

## Token format

Header `X-Heliolytics-Token`, format `ts.nonce.sig`:

```
ts    = unix seconds
nonce = 16 random bytes, hex
sig   = HMAC-SHA256(secret, "ts:nonce")
```

`internal/auth/signing.go` mints (`SignToken`) and verifies (`VerifyToken` / `VerifyTokenDetail`). `TokenVerifyResult` carries a `Reason` for log diagnosis without exposing the secret to the client. The HTTP response remains a bare 401.

### Implementations

| Repo | File |
|---|---|
| Go server | `internal/auth/signing.go` |
| Flutter app | `lib/services/network/heliolytics_token.dart` |
| Next.js web | `lib/api/signing.ts` |

The three implementations must use the same format. All three must agree byte-for-byte on the `"ts:nonce"` string being signed.

## Replay defense

Verification allows at most two seconds of future clock skew. This covers whole-second client timestamps arriving just before the server clock reaches the same second. Larger future offsets are rejected. Tokens more than five minutes old are also rejected.

Replay defense has two layers:

1. **Time window:** `TokenWindow = 5 * time.Minute`, with the bounded future tolerance above. A captured token expires quickly.
2. **Nonce store:** `internal/auth/nonce_store.go` remembers nonces within the window and rejects reuse. The window bounds memory because older nonces are evicted.

The nonce store and rate limiter use process memory, not a shared database. They do not provide distributed replay or rate-limit enforcement across instances.

The server validates the timestamp and signature before recording the nonce. An invalid signature cannot consume a nonce that a later valid request uses. After a valid signature is accepted, the nonce is consumed atomically and any replay is rejected.

Signature comparison uses `crypto/subtle` constant-time equality, not `==`.

## Rate limiting

`internal/middleware/ratelimit.go`, `RATE_LIMIT_PER_MIN` (default 120).

Client identity comes from the remote address unless `TRUST_PROXY=true`, in which case forwarded headers are honored. Without the flag behind a proxy, the server can rate-limit all clients as one address. With the flag and no trusted proxy, clients can spoof their identity through a header. Set it only when a trusted reverse proxy terminates traffic.

## Configuration

`internal/config/config.go` reads environment variables only. It does not read a config file.

| Var | Default | Notes |
|---|---|---|
| `ADDR` | `:8080` | Listen address |
| `HELIOLYTICS_SIGNING_SECRET` | *(empty)* | **Required.** Empty secret causes `SignToken` to return an error and verification to fail closed |
| `DATABASE_URL` | *(empty)* | Postgres DSN |
| `RATE_LIMIT_PER_MIN` | `120` | Per-client budget |
| `REPARSE_ENABLED` | `false` | Must be literal `"true"` |
| `REPARSE_SECRET` | *(empty)* | Separate secret for reparse |
| `TRUST_PROXY` | `false` | Honor forwarded client IP |

Secrets come from the environment. The repository ships placeholders only. Local-only deployment notes describe first-run setup.

## Reparse gate

`/api/v1/reparse` is disabled unless `REPARSE_ENABLED=true`. The code checks `X-Reparse-Secret` only when `REPARSE_SECRET` is non-empty. Configure an independent secret before enabling the route. The handler replays the latest stored session. It is not a full-history migration runner. The normal HMAC check still applies.

## Key files

| File | Role |
|---|---|
| `internal/auth/signing.go` | Mint and verify |
| `internal/auth/nonce_store.go` | Replay rejection |
| `internal/middleware/hmac.go` | Auth middleware |
| `internal/middleware/ratelimit.go` | Rate limiter |
| `internal/middleware/logging.go` | Request log |
| `internal/config/config.go` | Environment config |

Validation: not run for this revision.
