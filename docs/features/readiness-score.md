# Readiness and recovery

Source review: 7 September 2026. These are application rules, not a clinical assessment.

## Two different sources

The daily metric query uses `COALESCE(readiness, computed_readiness)`: device
readiness from `0x39` takes precedence; the Go calculation is a fallback.
The standalone `/api/v1/recovery` route always runs the Go calculation. Neither
current UI calls it. Its components are not an explanation of a device score.

## Inputs to the calculated fallback

`internal/readiness/readiness.go` receives a slice of `DayVitals`, oldest first.
It treats the last row as the target. `internal/store/readiness.go` loads up to
60 stored daily rows ending on or before the requested day.

| Component | Weight | Calculation |
|---|---|---|
| HRV | 0.50 | Deviation of ln(RMSSD) from prior values; higher increases subscore |
| Resting HR | 0.25 | Deviation from prior values; higher decreases subscore |
| Breathing | 0.10 | Deviation from prior values; higher decreases subscore |
| Sleep | 0.15 | Device sleep score clamped to 0-100, used directly |

Sleep is not baseline-normalized. HRV must be present and positive on the target.
There must be at least two valid prior HRV values: prior plus target must reach
`MinDays = 3`. The target itself is excluded from baseline mean and spread.
Resting HR and breathing each require a target value and at least two prior values.
Missing optional components are omitted; dividing by the included weight total
renormalizes the final weighted score. Returned component weights are the original
weights, not pre-normalized percentages.

## Mean and spread

For each baseline-based component, the mean uses up to seven prior valid values.
Below fourteen prior values for that component, the calculation uses a provisional
spread. At fourteen or more, it uses sample standard deviation over up to sixty
prior values with a lower bound to avoid overreacting to tiny changes.

These are counts of available values, not a guarantee of consecutive calendar
days. The store loads at most sixty daily rows including the target, so the
function may receive fewer prior values than its own sixty-value limit.

```text
z = (target - mean of recent prior values) / spread
subscore = clamp(50 + direction * 25 * z, 0, 100)
score = round(sum(subscore * weight) / sum(included weights))
```

Direction is +1 for log-HRV and -1 for resting HR/breathing.
Provisional spreads are 0.18 for log-HRV, 4 bpm for resting HR and 1 breath/min
for breathing. Personal spread floors are 0.08, 1.5 and 0.5 respectively.

## Storage and limits

Calculated readiness runs after the daily rollups in normal ingest. It writes
`computed_readiness`, not the device column. Insufficient input skips that update;
existing stored values are not explicitly cleared by that path.
The recovery handler returns a null score and `buildingBaseline` when calculation
reports insufficient input.

Because history is selected with `day_key <= requested day`, the standalone
handler can calculate from an earlier last row when the requested date has no row.
Do not describe it as verifying the existence of the requested day's observations.

Changing weights changes score meaning. Existing stored values do not magically
recompute everywhere after a source edit. Review the recalculation scope before
changing the formula.

See `internal/readiness/readiness_test.go` for formula examples. Those tests were
not rerun during this documentation update.
