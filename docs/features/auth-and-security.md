# Authentication and security

The API has no user login. Two fixed credentials authenticate signed requests:
`phone` may read and write, `web` may only read. `/health` and `/version` are
public. This split keeps a server-side web process from holding a write
credential. It is not role-based access control and it does not prove that a
request came from a specific handset.

## Signed request

Two headers carry the credential:

```text
X-Heliolytics-Auth: <kid>.<ts>.<nonce>.<mac>
X-Heliolytics-Content-SHA256: <sha256 of the exact body bytes>
```

- `kid` is `phone` or `web`.
- `ts` is integer Unix seconds and is accepted when
  `now-300 <= ts <= now+60`, both edges inclusive.
- `nonce` is 16 random bytes written as 32 lowercase hex characters.
- The body hash is the SHA-256 of the finalized body. An empty body hashes to
  `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.
- `mac` is HMAC-SHA256 over the canonical text, keyed by that kid's 32-byte
  secret, written as 64 lowercase hex characters.

Both headers must appear exactly once. A duplicate is rejected.

## Canonical text

Eight fields joined by a single newline, in this order:

```text
<kid>
<ts>
<nonce>
<METHOD>
<escapedPath>
<rawQuery>
<declared body hash>
<Content-Type>
```

The method is upper-cased by the server. There is no trailing newline after the
last field, so an empty Content-Type leaves an empty final field. The raw query
is signed verbatim: reordering parameters or adding one breaks the MAC. The
Content-Type is signed with its boundary, so a multipart body must be hashed after
it is finalized and must never be reserialized afterwards. `internal/auth/signing.go`
builds and verifies this text, and `internal/auth/testdata/signing.json` holds
the five published vectors that the Go, TypeScript and Dart clients check against.

## Replay defense

Consumed nonces live in the `auth_nonces` table with key `(kid, nonce)` and an
`expires_at` of `ts + 301`, the first second at which that timestamp can no longer
authenticate. Consumption is one atomic insert that reuses a row only after it has
expired, so exactly one concurrent request wins and every other one receives 401.
The nonce is consumed last: a request that fails a signature, permission, body or
structural check never records one.

`cmd/server` deletes expired rows every ten minutes. That purge only bounds the
table; replay rejection never depends on the ticker. Because the rows are in the
database, a restart, a second instance or a second connection pool cannot restore
replay eligibility.

## Verification order

`internal/middleware/hmac.go` runs one ordered pipeline:

1. Cheap header presence and shape.
2. Per-client rate limit.
3. MAC over the canonical text and the declared body hash.
4. Permission: a `web` request that is not `GET` gets 403 `forbidden`, before the
   body or the nonce is touched.
5. Per-kid rate limit.
6. Body slot, bounded read and digest check.
7. Route preparation and nonce consumption.
8. Handler.

Responses are JSON. A missing or malformed header, an unknown kid, a timestamp
outside the window, a MAC mismatch or a body-hash mismatch is 401 `unauthorized`;
the reason stays in the server log. A rate limit is 429 `rate_limited` with
`Retry-After`. A body slot that is busy is 503 `server_busy` with `Retry-After: 1`.
A body that stalls past the request deadline is 503 `request_timeout`, and a body
above the route cap is 413 `payload_too_large`.

## Rate limits

`RATE_LIMIT_PER_MIN` (default 120) applies to every request, before
authentication, keyed by client identity. `RATE_LIMIT_PER_KEY_PER_MIN` (default
600) applies per kid after the MAC verifies. Both are in-process token buckets
with a bounded key table, so they protect one instance and are not a distributed
limit.

Client identity is the immediate socket peer. `X-Forwarded-For` and `X-Real-IP`
are never read. With `TRUST_PROXY=true` the server reads `CF-Connecting-IP`, and
only when that peer address falls inside `TRUSTED_PROXY_CIDRS`. A trusted peer
that sends a missing, duplicated or invalid forwarding header gets 400
`invalid_proxy_header`. Set the flag only behind the trusted proxy.

## Body limits

Ingest accepts at most 16 MiB and profile at most 64 KiB. At most
`MAX_CONCURRENT_BODY_READS` uploads are read at once. Only a `phone` write reaches
the read step at all.

## Credentials and configuration

`HELIOLYTICS_API_KEYS` holds exactly `web.<64 lowercase hex>` and
`phone.<64 lowercase hex>`, in either order, with two distinct secrets. An empty
entry, an unknown kid, a repeated kid, a non-hex secret or the published example
secret fails startup: the server stores digests of known published and
development secrets and refuses them.

`internal/config/config.go` reads the environment once and returns an error naming
the variable. It never prints a value, never falls back on a malformed value and
never terminates the process itself. Build metadata is not environment
configuration: `commit` and `builtAt` come from linker flags, so `/version`
reports null when a build supplied neither.

| Variable | Default | Notes |
|---|---|---|
| `DATABASE_URL` | none | Required |
| `HELIOLYTICS_API_KEYS` | none | Required, exact grammar above |
| `ADDR` | `:8080` | Listen address; an explicit empty value is an error |
| `RATE_LIMIT_PER_MIN` | `120` | Per-client token bucket |
| `RATE_LIMIT_PER_KEY_PER_MIN` | `600` | Per-kid token bucket |
| `REQUEST_TIMEOUT_SECONDS` | `90` | Integer 1..90, the handler deadline |
| `MAX_CONCURRENT_BODY_READS` | `2` | Visitor slots for body reads |
| `TRUST_PROXY` | `false` | Exactly `true` or `false` |
| `TRUSTED_PROXY_CIDRS` | none | Comma-separated CIDRs; required when `TRUST_PROXY=true` |

The API reads no session secret; the web application owns its own. There is no
reparse configuration.

## Socket budgets

`cmd/server` sets a 5 s read-header timeout, 95 s read and write timeouts, 60 s
idle timeout and 16 KiB header limit. The request context timeout (1..90 s) stays
inside the write deadline, which stays below the edge budget. Shutdown is
signal-driven, waits for in-flight requests, stops the purge loop and then closes
the pool.

## Key files

| File | Role |
|---|---|
| `internal/auth/signing.go` | Canonical text, header parsing, MAC verification, key set |
| `internal/store/auth_nonces.go` | Atomic nonce consumption and expiry purge |
| `internal/middleware/hmac.go` | Ordered guard pipeline |
| `internal/middleware/ratelimit.go` | Client identity, token buckets, body slots |
| `internal/middleware/timeout.go` | Handler deadline and 503 response |
| `internal/config/config.go` | Environment configuration |

Validation: not run for this revision.
