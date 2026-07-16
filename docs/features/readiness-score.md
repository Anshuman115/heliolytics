# Feature — Readiness / recovery score

A daily 0–100 recovery score from nightly vitals. `internal/readiness/readiness.go`.

The strap reports its own readiness (type code `0x39`). This is the fallback when it
doesn't — and the app prefers the device value when present.

## Method

Baseline-deviation z-scoring, the standard approach in HRV-guided training. Each
metric is compared against **the user's own history**, not a population norm.

```
sub-score = clamp(50 ± 25·z, 0, 100)
```

So ±2 SD spans the full range, and the ~0.5 SD "smallest worthwhile change" moves
the score about 12 points — small enough to be honest, large enough to notice.

## Weights

```go
wHRV   = 0.50
wRHR   = 0.25
wSleep = 0.15
wResp  = 0.10
```

HRV-dominant by design. **Missing optional components are dropped and the remaining
weights renormalized** — a night without respiratory rate still scores, it just
reweights the rest.

## The four decisions worth knowing

**1. HRV uses `ln(RMSSD)`, not raw RMSSD.** RMSSD is log-normal; averaging it raw
skews the baseline. Baseline = 7-day rolling mean (`meanWindow`), spread = 60-day SD
(`sdWindow`).

**2. The baseline is prior days only.** The target day is never folded into its own
mean/SD — otherwise a bad day partly defines its own "normal" and the score
self-cancels.

**3. Vitals are sampled over the sleep window, not the calendar day.** Daytime HRV is
noise dominated by posture, caffeine, and stress. This happens upstream in the parse
aggregation, but it's the reason the score means anything.

**4. Cold start is explicit.** Below `MinDays = 3` valid HRV nights → **no score**,
reported as "building baseline" rather than a fabricated number.

## Provisional vs. personal SD

Between 3 and `fullSDDays = 14` nights, each metric uses a **population-prior SD**
instead of the user's own — a personal SD from 4 samples is noise.

| Constant | Value | Metric |
|---|---|---|
| `priorSDH` | 0.18 | ln(RMSSD) |
| `priorSDR` | 4.0 | RHR, bpm |
| `priorSDF` | 1.0 | Resp, br/min |
| `floorSDH` | 0.08 | Minimum ln(RMSSD) SD |
| `floorSDR` | 1.5 | Minimum RHR SD |
| `floorSDF` | 0.5 | Minimum resp SD |

The **floors** matter: a user with unusually stable vitals would otherwise get a
tiny SD, turning trivial fluctuations into huge z-scores and a score that swings
wildly for no reason.

## Input

```go
type DayVitals struct {
    RMSSD      *float64 // nightly mean RMSSD, ms
    RHR        *float64 // bpm
    Resp       *float64 // breaths/min
    SleepScore *float64 // 0–100
}
```

Pointers, because `nil` means "not available" and must be distinguishable from zero.

## Storage

Computed scores persist via `internal/store/readiness.go` (migration
`009_computed_readiness.sql`), so the app reads a value rather than recomputing.

## Tests

`internal/readiness/readiness_test.go` (unit) and
`internal/parse/dump_readiness_test.go` (against real captured dumps).

## If you change the weights

They're load-bearing and tuned. Changing them silently rewrites the meaning of every
historical score the user has already seen — scores stored before the change won't
match scores computed after. Re-parse deliberately or not at all.
