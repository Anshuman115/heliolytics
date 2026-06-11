package parse

type DayAcc struct {
	Steps         int
	Pai           *int
	Readiness     *int
	Spo2Sum       int
	Spo2Count     int
	HrvSum        int
	HrvCount      int
	RestingHr     *int
	MaxHr         *int
	RespRateSum   int
	RespRateCount int
	StressSum     int
	StressCount   int
	SleepScore    *int
	SleepMins     int
	SleepDeep     int
	SleepRem      int
	SleepLight    int
	TempSum       float64
	TempCount     int
	NapCount             int
	WorkoutCount         int
	ActivitySessionCount int
}

func (d *DayAcc) TempAvg() *float64 {
	if d.TempCount == 0 {
		return nil
	}
	v := d.TempSum / float64(d.TempCount)
	return &v
}

func (d *DayAcc) Spo2Avg() *int {
	if d.Spo2Count == 0 {
		return nil
	}
	v := d.Spo2Sum / d.Spo2Count
	return &v
}

func (d *DayAcc) HrvAvg() *int {
	if d.HrvCount == 0 {
		return nil
	}
	v := d.HrvSum / d.HrvCount
	return &v
}

func (d *DayAcc) StressAvg() *int {
	if d.StressCount == 0 {
		return nil
	}
	v := d.StressSum / d.StressCount
	return &v
}

func (d *DayAcc) RespRateAvg() *int {
	if d.RespRateCount == 0 {
		return nil
	}
	v := d.RespRateSum / d.RespRateCount
	return &v
}
