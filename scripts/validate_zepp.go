// scripts/validate_zepp.go
// Cross-validates Helio BLE binary dumps against Zepp Health CSV export.
package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	_dumpDir = "../Heliolytics_App/helio_dump_v3/"
	_zeppDir = "../Heliolytics_App/zepp-export/"
)

func main() {
	var dumpDir, zeppDir string
	flag.StringVar(&dumpDir, "dump", _dumpDir, "Path to helio_dump_v3/ directory")
	flag.StringVar(&zeppDir, "zepp", _zeppDir, "Path to zepp-export/ directory")
	flag.Parse()

	v := &Validator{dumpDir: dumpDir, zeppDir: zeppDir}

	fmt.Println("===============================================================")
	fmt.Println("  HELIO BLE/ZEPP CROSS-VALIDATION REPORT")
	fmt.Println("===============================================================")
	v.validateAll()
	fmt.Println("\n===============================================================")
	fmt.Println("  VALIDATION COMPLETE")
	fmt.Println("===============================================================")
}

type Validator struct {
	dumpDir string
	zeppDir string
}

func (v *Validator) validateAll() {
	v.validate0x01()
	v.validate0x48()
	v.validate0x4A()
	v.validate0x13()
	v.validate0x2E()
	v.validate0x38()
	v.validate0x39()
	v.validate0x3A()
	v.validate0x49()
	v.validate0x4E()
	v.validate0x55()
	v.validate0x57()
	v.validate0x06()
	v.validate0x25()
	v.validate0x26()
}

func (v *Validator) loadCSV(name string) ([][]string, error) {
	f, err := os.Open(filepath.Join(v.zeppDir, name))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return csv.NewReader(f).ReadAll()
}

func (v *Validator) loadBin(code byte) ([]byte, error) {
	lookup := map[byte]string{
		0x01: "0x01_hr_samples.bin",
		0x05: "0x05_workouts.bin",
		0x06: "0x06_workout_details.bin",
		0x0D: "0x0D_pai_scores.bin",
		0x13: "0x13_stress_auto.bin",
		0x25: "0x25_spo2.bin",
		0x26: "0x26_spo2_sleep.bin",
		0x27: "0x27_accelerometer.bin",
		0x2E: "0x2E_temperature.bin",
		0x38: "0x38_respiratory.bin",
		0x39: "0x39_readiness.bin",
		0x3A: "0x3A_resting_hr.bin",
		0x48: "0x48_sleep_minute.bin",
		0x49: "0x49_hrv_rmssd.bin",
		0x4A: "0x4A_hrv_trend.bin",
		0x4E: "0x4E_sleep_segments.bin",
		0x55: "0x55_rr_interval.bin",
		0x57: "0x57_hrv_latest.bin",
	}
	if fn, ok := lookup[code]; ok {
		return os.ReadFile(filepath.Join(v.dumpDir, fn))
	}
	return nil, fmt.Errorf("no mapping for code 0x%02x", code)
}

func abs(a int) int { if a < 0 { return -a }; return a }

// ACTIVITY (0x01)
func (v *Validator) validate0x01() {
	fmt.Println("\n-- Type 0x01: Activity Summary --")
	bin, err := v.loadBin(0x01)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠ Binary missing: %v\n", err)
		return
	}
	csvRows, err := v.loadCSV("ACTIVITY/ACTIVITY_1780920536462.csv")
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠ ACTIVITY.csv missing: %v\n", err)
		return
	}
	zepp := make(map[string]int)
	for _, r := range csvRows[1:] {
		if len(r) < 2 { continue }
		s, _ := strconv.Atoi(r[1])
		zepp[r[0]] = s
	}
	// TODO: aggregate steps from binary per day
	_ = zepp
	fmt.Printf("  ✓ Loaded %d bytes. Cross-checking requires daily timestamp aggregation.\n", len(bin))
}

// SLEEP (0x48)
func (v *Validator) validate0x48() {
	fmt.Println("\n-- Type 0x48: Sleep Sessions --")
	bin, err := v.loadBin(0x48)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠ Binary missing: %v\n", err)
		return
	}
	fmt.Printf("  ✓ %d bytes of sleep session data\n", len(bin))
}

// SLEEP SUMMARY (0x4A)
func (v *Validator) validate0x4A() {
	fmt.Println("\n-- Type 0x4A: Sleep Total Duration --")
	bin, err := v.loadBin(0x4A)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠ Binary missing: %v\n", err)
		return
	}

	f, err := os.Open(filepath.Join(v.zeppDir, "SLEEP/SLEEP_1780920536851.csv"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠ SLEEP.csv missing: %v\n", err)
		return
	}
	defer f.Close()

	zeppTotals := make(map[time.Time]int)
	r := csv.NewReader(f)
	r.LazyQuotes = true
	rows, err := r.ReadAll()
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠ CSV parse error, using manual: %v\n", err)
		f.Seek(0, 0)
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			parts := strings.SplitN(line, ",", 8)
			if len(parts) < 7 || parts[0] == "date" { continue }
			var d, s, w, rem int
			fmt.Sscanf(parts[1], "%d", &d)
			fmt.Sscanf(parts[2], "%d", &s)
			fmt.Sscanf(parts[3], "%d", &w)
			fmt.Sscanf(parts[6], "%d", &rem)
			date, _ := time.Parse("2006-01-02", parts[0])
			zeppTotals[date] = d + s + w + rem
		}
	} else {
		for i, row := range rows {
			if i == 0 || len(row) < 7 { continue }
			var d, s, w, rem int
			fmt.Sscanf(row[1], "%d", &d)
			fmt.Sscanf(row[2], "%d", &s)
			fmt.Sscanf(row[3], "%d", &w)
			fmt.Sscanf(row[6], "%d", &rem)
			date, _ := time.Parse("2006-01-02", row[0])
			zeppTotals[date] = d + s + w + rem
		}
	}

	const periodSize = 2896
	matches, mismatches := 0, 0
	for offset := 0; offset+6 <= len(bin); offset += periodSize {
		// midnight_ts (u32 LE) at offset 0
		midnightUnix := uint32(bin[offset]) | uint32(bin[offset+1])<<8 | uint32(bin[offset+2])<<16 | uint32(bin[offset+3])<<24
		midnight := time.Unix(int64(midnightUnix), 0).UTC()
		// total_sleep_sec (u16 LE) at offset 4
		sleepSec := int(bin[offset+4]) | int(bin[offset+5])<<8
		sleepMin := sleepSec / 60
		if zTotal, ok := zeppTotals[midnight]; ok {
			if abs(zTotal-sleepMin) <= 5 {
				matches++
			} else {
				mismatches++
				fmt.Printf("  ⚠ %s: helio=%d min, zepp=%d min (diff=%d)\n",
					midnight.Format("2006-01-02"), sleepMin, zTotal, zTotal-sleepMin)
			}
		}
	}
	fmt.Printf("  ✓ Matches: %d, Mismatches: %d\n", matches, mismatches)
}

// STRESS (0x13)
func (v *Validator) validate0x13() {
	fmt.Println("\n-- Type 0x13: Stress --")
	bin, err := v.loadBin(0x13)
	if err != nil { fmt.Fprintf(os.Stderr, "  ⚠ %v\n", err); return }
	fmt.Printf("  ✓ %d minutes of stress data\n", len(bin)/4)
	fmt.Printf("    Source: Zepp does NOT export stress\n")
}

// SKIN TEMP (0x2E)
func (v *Validator) validate0x2E() {
	fmt.Println("\n-- Type 0x2E: Skin Temperature --")
	bin, err := v.loadBin(0x2E)
	if err != nil { fmt.Fprintf(os.Stderr, "  ⚠ %v\n", err); return }
	fmt.Printf("  ✓ %d temperature records\n", len(bin)/8)
	fmt.Printf("    Source: Zepp does NOT export skin temperature\n")
}

// BREATHING RATE (0x38)
func (v *Validator) validate0x38() {
	fmt.Println("\n-- Type 0x38: Breathing Rate --")
	bin, err := v.loadBin(0x38)
	if err != nil { fmt.Fprintf(os.Stderr, "  ⚠ %v\n", err); return }
	fmt.Printf("  ✓ %d breathing rate records\n", len(bin)/8)
}

// READINESS (0x39)
func (v *Validator) validate0x39() {
	fmt.Println("\n-- Type 0x39: Readiness --")
	bin, err := v.loadBin(0x39)
	if err != nil { fmt.Fprintf(os.Stderr, "  ⚠ %v\n", err); return }
	fmt.Printf("  ✓ %d readiness records (6B each)\n", len(bin)/6)
	fmt.Printf("    Source: Zepp does NOT export readiness\n")
}

// RESTING HR / BODY BATTERY (0x3A)
func (v *Validator) validate0x3A() {
	fmt.Println("\n-- Type 0x3A: Resting HR / Body Battery --")
	bin, err := v.loadBin(0x3A)
	if err != nil { fmt.Fprintf(os.Stderr, "  ⚠ %v\n", err); return }
	fmt.Printf("  ✓ %d records (6B each)\n", len(bin)/6)
	fmt.Printf("    Source: Zepp does NOT export body battery\n")
}

// HRV (0x49)
func (v *Validator) validate0x49() {
	fmt.Println("\n-- Type 0x49: HRV (RMSSD) --")
	bin, err := v.loadBin(0x49)
	if err != nil { fmt.Fprintf(os.Stderr, "  ⚠ %v\n", err); return }
	fmt.Printf("  ✓ %d HRV records\n", len(bin)/6)
	fmt.Printf("    Source: Zepp does NOT export HRV\n")
}

// SLEEP SEGMENTS (0x4E)
func (v *Validator) validate0x4E() {
	fmt.Println("\n-- Type 0x4E: Sleep Segments --")
	bin, err := v.loadBin(0x4E)
	if err != nil { fmt.Fprintf(os.Stderr, "  ⚠ %v\n", err); return }
	fmt.Printf("  ✓ %d sleep segments (9B each)\n", len(bin)/9)
}

// RR INTERVALS (0x55)
func (v *Validator) validate0x55() {
	fmt.Println("\n-- Type 0x55: RR Intervals (per-second) --")
	bin, err := v.loadBin(0x55)
	if err != nil { fmt.Fprintf(os.Stderr, "  ⚠ %v\n", err); return }
	const recSize = 5
	records := len(bin) / recSize
	if records > 0 {
		firstUnix := uint32(bin[0]) | uint32(bin[1])<<8 | uint32(bin[2])<<16 | uint32(bin[3])<<24
		lastUnix := uint32(bin[(records-1)*recSize]) | uint32(bin[(records-1)*recSize+1])<<8 | uint32(bin[(records-1)*recSize+2])<<16 | uint32(bin[(records-1)*recSize+3])<<24
		firstTs := time.Unix(int64(firstUnix), 0).UTC()
		lastTs := time.Unix(int64(lastUnix), 0).UTC()
		fmt.Printf("  ✓ %d records (%.1f days)\n", records, lastTs.Sub(firstTs).Hours()/24)
		fmt.Printf("    First: %s, RR=%d ms\n", firstTs.Format("2006-01-02 15:04:05"), int(bin[4])*10)
		fmt.Printf("    Last:  %s\n", lastTs.Format("2006-01-02 15:04:05"))
	}
	fmt.Printf("    Source: Zepp does NOT export RR intervals\n")
}

// HRV SESSIONS (0x57)
func (v *Validator) validate0x57() {
	fmt.Println("\n-- Type 0x57: HRV Sessions (5-min blocks) --")
	bin, err := v.loadBin(0x57)
	if err != nil { fmt.Fprintf(os.Stderr, "  ⚠ %v\n", err); return }
	const sessionSize = 306
	sessions := len(bin) / sessionSize
	fmt.Printf("  ✓ %d sessions (%.1f hours)\n", sessions, float64(sessions*5)/60)
	fmt.Printf("    Source: Zepp does NOT export RR data\n")
}

// WORKOUTS (0x06)
func (v *Validator) validate0x06() {
	fmt.Println("\n-- Type 0x06: Workout Details --")
	bin, err := v.loadBin(0x06)
	if err != nil { fmt.Fprintf(os.Stderr, "  ⚠ %v\n", err); return }
	fmt.Printf("  ✓ %d bytes of workout data\n", len(bin))
	csvRows, _ := v.loadCSV("SPORT/SPORT_1780920538031.csv")
	if csvRows != nil {
		fmt.Printf("  ✓ %d workout records in Zepp export\n", len(csvRows)-1)
	}
	fmt.Printf("    ⚠ t06.bin is incomplete due to probe corruption during dump\n")
}

// SpO2 (0x25)
func (v *Validator) validate0x25() {
	fmt.Println("\n-- Type 0x25: SpO2 --")
	bin, err := v.loadBin(0x25)
	if err != nil { fmt.Fprintf(os.Stderr, "  ⚠ %v\n", err); return }
	fmt.Printf("  ✓ %d bytes of SpO2 data\n", len(bin))
	fmt.Printf("    Source: Zepp does NOT export SpO2\n")
}

// SpO2 SESSIONS (0x26)
func (v *Validator) validate0x26() {
	fmt.Println("\n-- Type 0x26: SpO2 Sessions --")
	bin, err := v.loadBin(0x26)
	if err != nil { fmt.Fprintf(os.Stderr, "  ⚠ %v\n", err); return }
	fmt.Printf("  ✓ %d bytes (%d sessions)\n", len(bin), len(bin)/30)
	fmt.Printf("    Source: Zepp does NOT export SpO2\n")
}
