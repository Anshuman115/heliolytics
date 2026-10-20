package ingest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Anshuman115/heliolytics/internal/store"
	"github.com/jackc/pgx/v5"
)

var profileDay = store.Day(2026, 10, 19)

func seedLoadDay(t *testing.T, st *store.Store) {
	t.Helper()
	ctx := context.Background()
	err := st.WithTx(ctx, func(tx pgx.Tx) error {
		if err := st.LockCanonical(ctx, tx); err != nil {
			return err
		}
		_, err := st.WriteSession(ctx, tx, store.SessionMeta{
			ID:        "profile-session",
			DeviceMAC: testMAC,
			StartedAt: profileDay,
			EndedAt:   profileDay.Add(time.Hour),
		}, []byte(`{"chunked":[]}`), []store.RawBlob{{TypeCode: "0x01", Payload: []byte{0x02}}})
		if err != nil {
			return err
		}
		sleep := store.SleepRow{
			StartedAt: profileDay,
			EndedAt:   profileDay.Add(3 * time.Hour),
			DayKey:    profileDay,
			Score:     new(70),
			TotalMins: 160,
			DeepMins:  60,
			RemMins:   50,
			LightMins: 50,
			WakeMins:  20,
		}
		if _, err := st.WriteSleep(ctx, tx, []store.SleepRow{sleep}, "profile-session", profileDay.Add(time.Hour), false); err != nil {
			return err
		}
		hr := make([]store.HeartRateSample, 0, 720)
		for i := range 720 {
			hr = append(hr, store.HeartRateSample{
				At:     profileDay.Add(time.Duration(i) * time.Minute),
				Source: "0x01",
				BPM:    100,
			})
		}
		_, err = st.WriteHeartRate(ctx, tx, hr, "profile-session", profileDay.Add(time.Hour))
		return err
	})
	if err != nil {
		t.Fatalf("seed load day: %v", err)
	}
}

func TestUpdateProfileNameOnlyCreatesNoDailyRows(t *testing.T) {
	st := openStore(t)
	profile := store.Profile{Name: new("Local Test")}
	if err := UpdateProfile(context.Background(), st, &profile); err != nil {
		t.Fatalf("update profile: %v", err)
	}

	saved, err := st.GetProfile(context.Background())
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if saved.Name == nil || *saved.Name != "Local Test" {
		t.Fatalf("name = %v, want Local Test", saved.Name)
	}
	days, err := st.ListDays(context.Background(), "2026-10-01", "2026-10-20")
	if err != nil {
		t.Fatalf("list days: %v", err)
	}
	if len(days) != 0 {
		t.Fatalf("daily rows = %d, want none from a profile alone", len(days))
	}
}

func TestUpdateProfileDOBAndSexRecomputeLoad(t *testing.T) {
	st := openStore(t)
	seedLoadDay(t, st)
	ctx := context.Background()

	birth := store.Day(1990, 1, 1)
	profile := store.Profile{BirthDate: &birth, Sex: new("male")}
	if err := UpdateProfile(ctx, st, &profile); err != nil {
		t.Fatalf("update profile: %v", err)
	}
	days, err := st.ListDays(ctx, "2026-10-19", "2026-10-19")
	if err != nil {
		t.Fatalf("list days: %v", err)
	}
	if len(days) != 1 {
		t.Fatalf("daily rows = %d, want 1", len(days))
	}
	if days[0].TrainingLoad == nil {
		t.Fatalf("load = nil, want a computed TRIMP; components = %s", days[0].ReadinessComponents)
	}

	cleared := store.Profile{}
	if err := UpdateProfile(ctx, st, &cleared); err != nil {
		t.Fatalf("clear profile: %v", err)
	}
	days, err = st.ListDays(ctx, "2026-10-19", "2026-10-19")
	if err != nil {
		t.Fatalf("list days: %v", err)
	}
	if len(days) != 1 {
		t.Fatalf("daily rows = %d, want 1", len(days))
	}
	if days[0].TrainingLoad != nil {
		t.Fatalf("load = %v, want NULL after clearing sex and birth date", *days[0].TrainingLoad)
	}
}

func TestUpdateProfileRejectsInvalidSex(t *testing.T) {
	st := openStore(t)
	profile := store.Profile{Sex: new("other")}
	err := UpdateProfile(context.Background(), st, &profile)
	var invalid *store.InvalidRowError
	if !errors.As(err, &invalid) {
		t.Fatalf("error = %v, want an invalid profile row", err)
	}
	saved, err := st.GetProfile(context.Background())
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if saved.Sex != nil {
		t.Fatalf("stored sex = %v, want the rejected write rolled back", *saved.Sex)
	}
}
