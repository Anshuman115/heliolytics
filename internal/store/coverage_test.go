package store

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestCoverageAlwaysCarriesMeasuredCodes(t *testing.T) {
	st := newTestStore(t)
	coverage, err := st.GetCoverage(context.Background())
	if err != nil {
		t.Fatalf("coverage: %v", err)
	}
	if coverage.HasData || coverage.LastIngestAt != nil || coverage.DataThrough != nil {
		t.Fatalf("empty coverage = %+v", coverage)
	}
	if len(coverage.Types) != len(MeasuredTypeCodes) {
		t.Fatalf("coverage keys = %d, want %d", len(coverage.Types), len(MeasuredTypeCodes))
	}
	for _, code := range MeasuredTypeCodes {
		value, ok := coverage.Types[code]
		if !ok {
			t.Fatalf("coverage missing key %s", code)
		}
		if value != nil {
			t.Fatalf("coverage %s = %v, want null", code, value)
		}
	}
	if _, ok := coverage.Types["0x26"]; ok {
		t.Fatal("coverage must not expose 0x26")
	}
}

func TestCoverageTracksExactCanonicalMaxima(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	day := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	sessionID := "coverage-canonical"
	observedAt := day.Add(24 * time.Hour)
	seedSession(t, st, sessionID, day, observedAt)

	hrAt := day.Add(4 * time.Hour)
	stepAt := day.Add(5 * time.Hour)
	stressAt := day.Add(6 * time.Hour)
	tempAt := day.Add(7 * time.Hour)
	hrvAt := day.Add(8 * time.Hour)
	respAt := day.Add(9 * time.Hour)
	spo2At := day.Add(10 * time.Hour)
	sleepEnd := day.Add(11 * time.Hour)
	workoutEnd := day.Add(12 * time.Hour)
	detailEnd := day.Add(13 * time.Hour)
	paiAt := day.Add(14 * time.Hour)

	mustTx(t, st, func(tx pgx.Tx) error {
		if _, err := st.WriteHeartRate(ctx, tx, []HeartRateSample{
			{At: hrAt, Source: "0x01", BPM: 60},
			{At: hrAt.Add(time.Minute), Source: "0x46", BPM: 120},
		}, sessionID, observedAt); err != nil {
			return err
		}
		if _, err := st.WriteSteps(ctx, tx, []StepSample{{At: stepAt, Steps: 0}}, sessionID, observedAt); err != nil {
			return err
		}
		if _, err := st.WriteStress(ctx, tx, []StressSample{{At: stressAt, Stress: 30}}, sessionID, observedAt); err != nil {
			return err
		}
		if _, err := st.WriteTemperature(ctx, tx, []TemperatureSample{{At: tempAt, CentiC: 3627}}, sessionID, observedAt); err != nil {
			return err
		}
		if _, err := st.WriteHRV(ctx, tx, []HRVSample{{At: hrvAt, RMSSDMS: 50}}, sessionID, observedAt); err != nil {
			return err
		}
		if _, err := st.WriteResp(ctx, tx, []RespSample{{At: respAt, BRMin: 15}}, sessionID, observedAt); err != nil {
			return err
		}
		if _, err := st.WriteSpO2(ctx, tx, []SpO2Sample{
			{At: spo2At, Source: "0x25", Mode: "auto", Pct: 97},
		}, sessionID, observedAt); err != nil {
			return err
		}
		if _, err := st.WriteSleep(ctx, tx, []SleepRow{
			sleepCandidate(day.Add(2*time.Hour), sleepEnd),
		}, sessionID, observedAt, false); err != nil {
			return err
		}
		if _, err := st.WriteWorkouts(ctx, tx, []ActivityRow{
			workoutCandidate(day.Add(11*time.Hour), workoutEnd),
		}, sessionID, observedAt, false); err != nil {
			return err
		}
		if _, err := st.WritePAI(ctx, tx, []PAIScore{
			{DayKey: Day(2026, time.October, 1), SampledAt: paiAt, Score: 80},
		}, sessionID, observedAt); err != nil {
			return err
		}
		return st.RefreshTypeCoverage(ctx, tx, MeasuredTypeCodes)
	})

	coverage, err := st.GetCoverage(ctx)
	if err != nil {
		t.Fatalf("coverage: %v", err)
	}
	want := map[string]*time.Time{
		"0x01": new(stepAt),
		"0x05": new(workoutEnd),
		"0x06": nil,
		"0x0D": new(paiAt),
		"0x13": new(stressAt),
		"0x25": new(spo2At),
		"0x2E": new(tempAt),
		"0x38": new(respAt),
		"0x46": new(hrAt.Add(time.Minute)),
		"0x48": new(sleepEnd),
		"0x49": new(hrvAt),
	}
	for code, expected := range want {
		got := coverage.Types[code]
		switch {
		case expected == nil && got != nil:
			t.Fatalf("coverage %s = %v, want null", code, got)
		case expected != nil && got == nil:
			t.Fatalf("coverage %s = null, want %v", code, expected)
		case expected != nil && !got.Equal(*expected):
			t.Fatalf("coverage %s = %v, want %v", code, got, expected)
		}
	}
	if !coverage.HasData {
		t.Fatal("coverage hasData = false with canonical rows present")
	}
	if coverage.LastIngestAt == nil {
		t.Fatal("lastIngestAt missing after a stored session")
	}

	detail := workoutCandidate(day.Add(12*time.Hour+30*time.Minute), detailEnd)
	detail.HasDetail = true
	seedSession(t, st, "coverage-detail", day, observedAt.Add(time.Hour))
	mustTx(t, st, func(tx pgx.Tx) error {
		if _, err := st.WriteWorkouts(ctx, tx, []ActivityRow{detail}, "coverage-detail", observedAt.Add(time.Hour), false); err != nil {
			return err
		}
		return st.RefreshTypeCoverage(ctx, tx, []string{"0x05", "0x06"})
	})
	coverage, err = st.GetCoverage(ctx)
	if err != nil {
		t.Fatalf("coverage: %v", err)
	}
	if coverage.Types["0x06"] == nil || !coverage.Types["0x06"].Equal(detailEnd) {
		t.Fatalf("0x06 = %v, want the attached-detail end %v", coverage.Types["0x06"], detailEnd)
	}
}

func TestCoverageRawOnlyUploadLeavesHasDataFalse(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	seedSession(t, st, "raw-only", at, at.Add(time.Hour))

	coverage, err := st.GetCoverage(ctx)
	if err != nil {
		t.Fatalf("coverage: %v", err)
	}
	if coverage.HasData {
		t.Fatal("raw-only upload set hasData")
	}
	if coverage.LastIngestAt == nil {
		t.Fatal("raw-only upload must still advance lastIngestAt")
	}
}

func TestCoverageDeletesWatermarkWhenCanonicalRowsGoAway(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	day := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	observedAt := day.Add(time.Hour)
	seedSession(t, st, "coverage-delete", day, observedAt)

	mustTx(t, st, func(tx pgx.Tx) error {
		if _, err := st.WriteStress(ctx, tx, []StressSample{{At: day.Add(time.Hour), Stress: 30}}, "coverage-delete", observedAt); err != nil {
			return err
		}
		return st.RefreshTypeCoverage(ctx, tx, []string{"0x13"})
	})
	if coverage, err := st.GetCoverage(ctx); err != nil || coverage.Types["0x13"] == nil {
		t.Fatalf("0x13 watermark = %v, err %v", coverage.Types["0x13"], err)
	}

	if _, err := st.pool.Exec(ctx, `DELETE FROM stress_samples`); err != nil {
		t.Fatalf("delete samples: %v", err)
	}
	mustTx(t, st, func(tx pgx.Tx) error {
		return st.RefreshTypeCoverage(ctx, tx, []string{"0x13"})
	})
	coverage, err := st.GetCoverage(ctx)
	if err != nil {
		t.Fatalf("coverage: %v", err)
	}
	if coverage.Types["0x13"] != nil {
		t.Fatalf("0x13 = %v, want the watermark deleted", coverage.Types["0x13"])
	}
}

func TestSessionTypeDataThroughIsScopedToTheSession(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	day := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	oldObserved := day.Add(10 * time.Hour)
	newObserved := day.Add(20 * time.Hour)
	seedSession(t, st, "watermark-old", day, oldObserved)
	seedSession(t, st, "watermark-new", day, newObserved)

	mustTx(t, st, func(tx pgx.Tx) error {
		if _, err := st.WriteHeartRate(ctx, tx,
			[]HeartRateSample{{At: day.Add(time.Hour), Source: "0x01", BPM: 60}}, "watermark-old", oldObserved); err != nil {
			return err
		}
		if _, err := st.WriteHeartRate(ctx, tx,
			[]HeartRateSample{{At: day.Add(2 * time.Hour), Source: "0x01", BPM: 62}}, "watermark-new", newObserved); err != nil {
			return err
		}
		if _, err := st.WriteHeartRate(ctx, tx,
			[]HeartRateSample{{At: day.Add(2 * time.Hour), Source: "0x46", BPM: 90}}, "watermark-new", newObserved); err != nil {
			return err
		}
		return nil
	})

	mustTx(t, st, func(tx pgx.Tx) error {
		view := NewTxView(tx)
		got, err := view.SessionTypeDataThrough(ctx, "watermark-old", []string{"0x01", "0x46", "0x26"})
		if err != nil {
			return err
		}
		if got["0x01"] == nil || !got["0x01"].Equal(day.Add(time.Hour)) {
			t.Fatalf("old 0x01 = %v, want its own session maximum", got["0x01"])
		}
		if got["0x46"] != nil {
			t.Fatalf("old session 0x46 = %v, want null", got["0x46"])
		}
		if value, ok := got["0x26"]; !ok || value != nil {
			t.Fatalf("0x26 = %v present=%v, want a null key", value, ok)
		}
		return nil
	})

	coverage, err := st.GetCoverage(ctx)
	if err != nil {
		t.Fatalf("coverage: %v", err)
	}
	if coverage.Types["0x01"] != nil {
		t.Fatal("SessionTypeDataThrough must not update global coverage")
	}
}
