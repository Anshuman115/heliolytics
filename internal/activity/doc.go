// Package activity detects automatic activity sessions from canonical minute
// steps. It is pure: no I/O, no clock, no store, no decoder dependency.
//
// Algorithm: one ordered scan. A minute moves when Steps >= CadenceMin. Up to
// GapMax observed below-threshold buckets may sit between moving buckets;
// GapMax+1 closes the run. A missing bucket or a workout bucket always closes
// it. End is one minute after the last moving bucket, and a run is kept only
// when End-Start >= DurationMin minutes.
//
// Units: UTC minute-aligned timestamps, steps and cadence per minute, HR in
// BPM. AvgHR is half-up rounding of observed HR only, over the 25..250 values
// the caller supplies; a minute without HR is never counted as zero.
//
// Limitations: cadence misses cycling, strength training and wheelchair
// movement; an absent minute is not a known zero; Walk and Run are heuristic
// cadence labels, not recorded sports; there is no calorie or distance estimate.
package activity
