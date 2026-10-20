package store

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func putProfile(t *testing.T, st *Store, p Profile) {
	t.Helper()
	mustTx(t, st, func(tx pgx.Tx) error {
		return NewTxView(tx).PutProfile(context.Background(), p)
	})
}

func TestProfileEmptySingletonReadsAllNull(t *testing.T) {
	st := newTestStore(t)
	got, err := st.GetProfile(context.Background())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != nil || got.BirthDate != nil || got.Sex != nil || got.HeightCm != nil || got.WeightKg != nil {
		t.Fatalf("empty profile = %+v, want all null", got)
	}
}

func TestProfileFullReplacementAndClear(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	name := "Test User"
	birth := Day(1995, time.April, 12)
	sex := "male"
	height := 178.0
	weight := 74.5
	putProfile(t, st, Profile{Name: &name, BirthDate: &birth, Sex: &sex, HeightCm: &height, WeightKg: &weight})

	got, err := st.GetProfile(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name == nil || *got.Name != name {
		t.Fatalf("name = %v", got.Name)
	}
	if got.BirthDate == nil || !got.BirthDate.Equal(birth) {
		t.Fatalf("birth date = %v, want %v", got.BirthDate, birth)
	}
	if got.Sex == nil || *got.Sex != sex {
		t.Fatalf("sex = %v", got.Sex)
	}
	if got.HeightCm == nil || *got.HeightCm != height || got.WeightKg == nil || *got.WeightKg != weight {
		t.Fatalf("body metrics = %v/%v", got.HeightCm, got.WeightKg)
	}

	putProfile(t, st, Profile{})
	got, err = st.GetProfile(ctx)
	if err != nil {
		t.Fatalf("get after clear: %v", err)
	}
	if got.Name != nil || got.BirthDate != nil || got.Sex != nil || got.HeightCm != nil || got.WeightKg != nil {
		t.Fatalf("cleared profile = %+v, want all null", got)
	}
	if n := countRows(t, st, "profile"); n != 1 {
		t.Fatalf("profile rows = %d, want the singleton row", n)
	}
}

func TestProfileRejectsInvalidValues(t *testing.T) {
	st := newTestStore(t)
	sex := "other"
	if err := st.WithTx(context.Background(), func(tx pgx.Tx) error {
		return NewTxView(tx).PutProfile(context.Background(), Profile{Sex: &sex})
	}); err == nil {
		t.Fatal("invalid sex accepted")
	}

	tooHeavy := 400.0
	if err := st.WithTx(context.Background(), func(tx pgx.Tx) error {
		return NewTxView(tx).PutProfile(context.Background(), Profile{WeightKg: &tooHeavy})
	}); err == nil {
		t.Fatal("out of range weight accepted")
	}
}

func TestProfileBirthDateStoresCalendarDateOnly(t *testing.T) {
	st := newTestStore(t)
	birth := time.Date(1995, 4, 12, 18, 30, 0, 0, time.UTC)
	putProfile(t, st, Profile{BirthDate: &birth})

	got, err := st.GetProfile(context.Background())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.BirthDate == nil {
		t.Fatal("birth date missing")
	}
	if got.BirthDate.Format(dateLayout) != "1995-04-13" {
		t.Fatalf("IST calendar date of the instant = %s, want 1995-04-13",
			got.BirthDate.Format(dateLayout))
	}
}
