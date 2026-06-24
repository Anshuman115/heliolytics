package store

import "time"

// SyncedBLETypeCodes matches Heliolytics_App fetchTypeCodes — every key must appear in coverage.types.
var SyncedBLETypeCodes = []string{
	"0x01", "0x05", "0x06", "0x0D", "0x13", "0x25", "0x26", "0x2E",
	"0x38", "0x39", "0x3A", "0x3B", "0x3D", "0x46", "0x48", "0x49", "0x4E",
}

func buildTypeCoverage(
	workoutEnd, activityEnd, mainSleepEnd, napEnd, tempEnd, hrEnd *time.Time,
	stressEnd, hrvEnd, spo2End, spo2SleepEnd, respEnd, rhrEnd *time.Time,
	stepsEnd, paiEnd, readinessEnd *time.Time,
) map[string]*time.Time {
	return map[string]*time.Time{
		"0x01": stepsEnd,
		"0x05": workoutEnd,
		"0x06": workoutEnd,
		"0x0D": paiEnd,
		"0x13": stressEnd,
		"0x25": spo2End,
		"0x26": spo2SleepEnd,
		"0x2E": tempEnd,
		"0x38": respEnd,
		"0x39": readinessEnd,
		"0x3A": rhrEnd,
		"0x3B": activityEnd,
		"0x3D": nil, // device max-HR: fetched by the app but not parsed/stored — no coverage
		"0x46": hrEnd,
		"0x48": mainSleepEnd,
		"0x49": hrvEnd,
		"0x4E": napEnd,
	}
}
