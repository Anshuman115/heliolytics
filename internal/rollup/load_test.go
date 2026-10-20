package rollup

import (
	"math"
	"testing"
	"time"

	"github.com/Anshuman115/heliolytics/internal/store"
)

func hrSeries(n, bpm int) []store.HeartRateSample {
	out := make([]store.HeartRateSample, 0, n)
	for i := range n {
		out = append(out, store.HeartRateSample{At: at(0, i), Source: "0x01", BPM: bpm})
	}
	return out
}

func TestTRIMPArithmeticMatchesCoefficients(t *testing.T) {
	birth := time.Date(1990, 1, 1, 0, 0, 0, 0, store.IST)
	hr := hrSeries(720, 100)
	hrRest := 60.0
	age := 36
	reserve := 208 - 0.7*float64(age) - hrRest
	r := (100 - hrRest) / reserve

	male := trimp(testDay, hr, &hrRest, &birth, "male")
	if male == nil {
		t.Fatal("male load must be available")
	}
	wantMale := 720 * 0.64 * r * math.Exp(1.92*r)
	if math.Abs(*male-wantMale) > 1e-9 {
		t.Fatalf("male trimp = %v, want %v", *male, wantMale)
	}

	female := trimp(testDay, hr, &hrRest, &birth, "female")
	if female == nil {
		t.Fatal("female load must be available")
	}
	wantFemale := 720 * 0.86 * r * math.Exp(1.67*r)
	if math.Abs(*female-wantFemale) > 1e-9 {
		t.Fatalf("female trimp = %v, want %v", *female, wantFemale)
	}
	if *male == *female {
		t.Fatal("the sex-specific coefficients must differ")
	}
}

func TestTRIMPReserveClamps(t *testing.T) {
	birth := time.Date(1990, 1, 1, 0, 0, 0, 0, store.IST)
	hrRest := 60.0

	low := trimp(testDay, hrSeries(720, 50), &hrRest, &birth, "male")
	if low == nil || *low != 0 {
		t.Fatalf("below-rest minutes = %v, want an exact zero", low)
	}

	high := trimp(testDay, hrSeries(720, 250), &hrRest, &birth, "male")
	if high == nil {
		t.Fatal("above-max minutes must stay finite and counted")
	}
	want := 720 * 0.64 * 1 * math.Exp(1.92)
	if math.Abs(*high-want) > 1e-9 {
		t.Fatalf("clamped reserve trimp = %v, want %v", *high, want)
	}
}

func TestTRIMPRequiresSevenHundredTwentyMinutes(t *testing.T) {
	birth := time.Date(1990, 1, 1, 0, 0, 0, 0, store.IST)
	hrRest := 60.0
	if trimp(testDay, hrSeries(719, 100), &hrRest, &birth, "male") != nil {
		t.Fatal("719 minutes must stay NULL")
	}
	got := trimp(testDay, hrSeries(720, 100), &hrRest, &birth, "male")
	if got == nil || *got <= 0 {
		t.Fatalf("720 minutes = %v, want a positive load", got)
	}
}

func TestTRIMPIntensityThreshold(t *testing.T) {
	birth := time.Date(1986, 1, 1, 0, 0, 0, 0, store.IST)
	hrRest := 80.0
	below := trimp(testDay, hrSeries(720, 109), &hrRest, &birth, "male")
	if below == nil || *below != 0 {
		t.Fatalf("reserve 0.29 = %v, want an exact zero", below)
	}
	at30 := trimp(testDay, hrSeries(720, 110), &hrRest, &birth, "male")
	want := 720 * 0.64 * 0.30 * math.Exp(1.92*0.30)
	if at30 == nil || math.Abs(*at30-want) > 1e-9 {
		t.Fatalf("reserve 0.30 = %v, want %v", at30, want)
	}
}

func TestTRIMPUnavailableInputs(t *testing.T) {
	birth := time.Date(1990, 1, 1, 0, 0, 0, 0, store.IST)
	hrRest := 60.0
	hr := hrSeries(720, 120)
	if trimp(testDay, hr, nil, &birth, "male") != nil {
		t.Fatal("missing resting HR must stay NULL")
	}
	if trimp(testDay, hr, &hrRest, nil, "male") != nil {
		t.Fatal("missing birth date must stay NULL")
	}
	if trimp(testDay, hr, &hrRest, &birth, "") != nil {
		t.Fatal("missing sex must stay NULL")
	}
	tooHigh := 200.0
	if trimp(testDay, hr, &tooHigh, &birth, "male") != nil {
		t.Fatal("a non-positive reserve must stay NULL")
	}
}

func TestCompletedAgeAndLeapBirthday(t *testing.T) {
	leap := time.Date(2000, 2, 29, 0, 0, 0, 0, store.IST)
	if got := completedAge(time.Date(2026, 2, 28, 0, 0, 0, 0, store.IST), leap); got != 25 {
		t.Fatalf("day before the anniversary = %d, want 25", got)
	}
	if got := completedAge(time.Date(2026, 3, 1, 0, 0, 0, 0, store.IST), leap); got != 26 {
		t.Fatalf("March 1 in a non-leap year = %d, want 26", got)
	}
	if got := completedAge(time.Date(2024, 2, 29, 0, 0, 0, 0, store.IST), leap); got != 24 {
		t.Fatalf("leap anniversary = %d, want 24", got)
	}
	if got := maxHeartRate(36); math.Abs(got-182.8) > 1e-9 {
		t.Fatalf("HRmax(36) = %v, want 182.8", got)
	}
}

func TestRestingHRBaselineMedianAndFallback(t *testing.T) {
	history := map[int]float64{
		dayKeyOf(testDay.AddDate(0, 0, -1)): 50,
		dayKeyOf(testDay.AddDate(0, 0, -2)): 60,
		dayKeyOf(testDay.AddDate(0, 0, -3)): 70,
	}
	got := restingHRBaseline(history, testDay, new(58.0))
	if got == nil || *got != 60 {
		t.Fatalf("baseline = %v, want the median 60", got)
	}

	thin := map[int]float64{
		dayKeyOf(testDay.AddDate(0, 0, -1)): 50,
		dayKeyOf(testDay.AddDate(0, 0, -2)): 60,
	}
	got = restingHRBaseline(thin, testDay, new(58.0))
	if got == nil || *got != 58 {
		t.Fatalf("thin baseline = %v, want the day's own 58", got)
	}
	if restingHRBaseline(thin, testDay, nil) != nil {
		t.Fatal("no baseline and no daily value must stay NULL")
	}
}
