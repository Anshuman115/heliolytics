# Readiness and recovery

Readiness is a Heliolytics product score computed from nightly vitals and the
previous day's training load. It is not a clinical assessment, a diagnosis or a
reproduction of any vendor score.

## Output model

Every canonical supported day gets one persisted explanation with a status, the
components in fixed order and the flags.

| Status | Meaning | Score |
|---|---|---|
| `scored` | At least 14 prior valid HRV nights and a valid baseline | 0-100 |
| `calibrating` | Valid HRV baseline with 3 to 13 prior nights | null |
| `no_baseline` | Tonight's HRV exists but its baseline does not | null |
| `no_data` | Tonight's HRV is missing | null |

The band is a display category only: `good` at 67 or more, `fair` at 34 to 66,
`low` at 33 or less. A missing score has no band.

## Inputs

Nightly values come from the same day's daily row:

- HRV: mean accepted `0x49` samples inside the sleep union, using
  `ln(rmssd_ms)` everywhere.
- Sleeping heart rate: the lowest qualifying five-minute mean.
- Respiration: mean accepted `0x38` samples inside the sleep union.
- Skin temperature: mean proxy-on-wrist sample in the sleep union.
- Sleep: the device sleep score of the longest session.
- Load: the previous day's TRIMP.

A missing input is null, never zero. The previous day's TRIMP and other
optional components can be absent without blocking the score.

## Baseline and spread

For each baseline component the prior values on `[D-60, D-1]` are used, with the
target day excluded.

- Baseline requires at least three values in total and at least three in
  `[D-28, D-1]`.
- Center is the median; even counts average the two middle values.
- Personal spread is `1.4826 * median(|value - center|)`.
- Used spread is
  `max(floor, sqrt((n * personal^2 + 7 * prior^2) / (n + 7)))`.

The shrinkage term keeps the spread continuous at every night count, so the
fourteenth night does not change the spread suddenly.

| Component | Prior | Floor |
|---|---|---|
| log HRV | 0.10 | 0.05 |
| Sleeping heart rate | 2.5 bpm | 1.0 |
| Respiration | 0.5 br/min | 0.25 |
| Skin temperature | 0.35 C | 0.15 |

## Components and weights

Order and base weight:

| Component | Weight | Sub-score |
|---|---|---|
| `hrv` | 0.30 | `clamp(50 + 25z, 0, 100)` |
| `hrv_balance` | 0.15 | Same, using the mean log HRV of `[D-6, D]` and half the HRV spread |
| `sleeping_hr` | 0.20 | `clamp(50 - 25z, 0, 100)` |
| `sleep` | 0.15 | `clamp(sleep_score, 0, 100)` |
| `skin_temp` | 0.10 | `clamp(50 - 25 * max(0, abs(z) - 1), 0, 50)` |
| `resp_rate` | 0.05 | `clamp(50 - 25z, 0, 100)` |
| `load` | 0.05 | `clamp(50 - 25 * (ratio - 1), 0, 75)` |

Higher HRV is always better, including above two standard deviations. There is
no high-HRV penalty. The sleep score is used as reported; chronic short sleep is
not renormalized into a neutral score.

HRV balance needs at least three valid nights in the seven-day window and a
valid HRV baseline. Load uses `ratio = trimp(D-1) / max(chronic_mean, 10)` with
at least 14 valid daily TRIMP values in `[D-28, D-1]`. A recorded zero-load day
is valid and reaches the 75 cap.

Present components renormalize their weights, so stored weights sum to 1 and
stored points sum to the raw score. In `no_data` and `no_baseline` states the
available components still carry their diagnostics and normalized weights, but
the top-level score and raw value stay null.

## Training load

TRIMP is the daily Banister-style sum over distinct canonical `0x01` minutes:

```text
age      = completed calendar years at the IST day
HRmax    = 208 - 0.7 * age
HRrest   = median prior resting HR, else the day's own
r        = clamp((HR - HRrest) / (HRmax - HRrest), 0, 1)
trimp    = sum of a * r * exp(b * r) for each minute with r >= 0.30
```

Male `(a, b)` is `(0.64, 1.92)`; female is `(0.86, 1.67)`. A February 29
birthday advances on March 1 in non-leap years.

TRIMP is null when the birth date, sex, resting HR baseline or a positive
reserve is missing, or when the day has fewer than 720 distinct `0x01` minutes.
A covered day whose minutes all fall below the threshold reports an exact 0.
Snapshot `0x46` heart rate never contributes. `0x3A` and `0x39` device values
are not used.

## Persistence and recomputation

Readiness runs inside the ingest, profile and rebuild transactions after the
daily aggregates and load.

- Only canonical supported dates get rows. Gaps and removed days are virtual
  missing inputs and are never persisted as `no_data` rows.
- The explanation always persists, including status, every component, the
  flags, `baselineNights` and the temperature deviation.
- A birth date or sex change, including a clear, recomputes every historical
  load and readiness row. Name, height and weight do not trigger a recompute.
- An older night change refreshes later resting HR baselines, so the whole
  affected suffix is recomputed rather than only the uploaded date.

## Flags

Flags appear in this order:

1. `missing:<component>` for absent inputs, in component order.
2. `baseline_stale:<component>` for present inputs without a usable baseline.
3. `temp_high` at `z >= 2`, `temp_low` at `z <= -2`.
4. `load_spike` when all seven acute days and the chronic baseline are valid and
   `mean7 / max(chronic, 10) >= 1.5`.
5. `low_coverage` when scored base weights sum to less than 0.70.

`load_spike` compares apparent acute strain only. It is not a validated injury
predictor.

## Limitations

The weights, priors, thresholds, shrinkage, gain, bands and coverage floors are
Heliolytics choices informed by published practice, not validated medicine.
There is no published universal industry-standard readiness formula. Changing
any of them requires a fresh-target test replay.

The `on_wrist` temperature flag is a project proxy: a matching canonical `0x01`
heart rate minute plus a plausible 30 to 40 C sample. The absence of heart rate
may mean missing device history and does not prove the strap was off the wrist.

Validation: not run for this revision.
