package parse

import "math"

type workoutSummary struct {
	startedAt   int64
	durationSec int64
	sportType   int
	calories    *int
	avgHR       *int
	maxHR       *int
	minHR       *int
}

func decodeWorkoutSummary(blob []byte, offset int) (*workoutSummary, int, error) {
	budget := &protoBudget{}
	top, err := readMessage(blob, 0, budget)
	if err != nil {
		return nil, 0, newError(ErrCodeMalformedProtobuf, typeWorkout, offset, "%v", err)
	}
	meta, err := subMessage(top, 2, 0, budget)
	if err != nil {
		return nil, 0, newError(ErrCodeMalformedProtobuf, typeWorkout, offset, "%v", err)
	}
	start, ok := varintField(meta, 1)
	if !ok || start > math.MaxInt64 {
		return nil, 0, nil
	}
	durationMessage, err := subMessage(top, 7, 0, budget)
	if err != nil {
		return nil, 0, newError(ErrCodeMalformedProtobuf, typeWorkout, offset, "%v", err)
	}
	duration, ok := varintField(durationMessage, 1)
	if !ok || duration > math.MaxInt64 {
		return nil, 0, nil
	}
	sport, err := workoutSport(top, budget)
	if err != nil {
		return nil, 0, newError(ErrCodeInvalidFraming, typeWorkout, offset, "%v", err)
	}
	caloriesMessage, err := subMessage(top, 16, 0, budget)
	if err != nil {
		return nil, 0, newError(ErrCodeMalformedProtobuf, typeWorkout, offset, "%v", err)
	}
	hrMessage, err := subMessage(top, 19, 0, budget)
	if err != nil {
		return nil, 0, newError(ErrCodeMalformedProtobuf, typeWorkout, offset, "%v", err)
	}

	dropped := 0
	caloriesValue, caloriesPresent := varintField(caloriesMessage, 1)
	calories, caloriesOut := statValue(caloriesValue, caloriesPresent, 0, 10000)
	if caloriesOut {
		dropped++
	}
	avgValue, avgPresent := varintField(hrMessage, 1)
	avgHR, avgOut := statValue(avgValue, avgPresent, hrMinBPM, hrMaxBPM)
	if avgOut {
		dropped++
	}
	maxValue, maxPresent := varintField(hrMessage, 2)
	maxHR, maxOut := statValue(maxValue, maxPresent, hrMinBPM, hrMaxBPM)
	if maxOut {
		dropped++
	}
	minValue, minPresent := varintField(hrMessage, 3)
	minHR, minOut := statValue(minValue, minPresent, hrMinBPM, hrMaxBPM)
	if minOut {
		dropped++
	}
	return &workoutSummary{
		startedAt:   int64(start),
		durationSec: int64(duration),
		sportType:   sport,
		calories:    calories,
		avgHR:       avgHR,
		maxHR:       maxHR,
		minHR:       minHR,
	}, dropped, nil
}

func statValue(value uint64, present bool, minimum, maximum int) (*int, bool) {
	if !present {
		return nil, false
	}
	if value < uint64(minimum) || value > uint64(maximum) {
		return nil, true
	}
	number := int(value)
	return &number, false
}
