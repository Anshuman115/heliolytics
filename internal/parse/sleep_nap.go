package parse

import "encoding/binary"

func isSleepStage(ty int) bool {
	return ty == 4 || ty == 5 || ty == 7 || ty == 8
}

func parseNightStages(raw []byte, off, count int, base int64) []SleepStage {
	var out []SleepStage
	limit := count
	if limit > 51 {
		limit = 51
	}
	for i := 0; i < limit && off+5*i+5 <= sleepRecordSize; i++ {
		s := int(binary.LittleEndian.Uint16(raw[off+5*i:]))
		e := int(binary.LittleEndian.Uint16(raw[off+5*i+2:]))
		ty := int(raw[off+5*i+4])
		if s == 0 && e == 0 {
			break
		}
		if !isSleepStage(ty) {
			continue
		}
		out = append(out, SleepStage{
			Start: EpochUTC(base + int64(s*60)),
			End:   EpochUTC(base + int64(e*60)),
			Type:  ty,
		})
	}
	return out
}

func sumStageMins(st []SleepStage, ty int) int {
	total := 0
	for _, g := range st {
		if g.Type == ty {
			total += int(g.End.Sub(g.Start).Minutes())
		}
	}
	return total
}

func parseNaps(raw []byte, base int64) []SleepRecord {
	var out []SleepRecord
	type window struct{ start, end int }
	var wins []window
	for p := 0x18; p+6 <= 0x54; p += 6 {
		ns := int(binary.LittleEndian.Uint16(raw[p:]))
		ne := int(binary.LittleEndian.Uint16(raw[p+2:]))
		nd := int(binary.LittleEndian.Uint16(raw[p+4:]))
		if ns == 0 && nd == 0 {
			break
		}
		if nd > 0 && ne > ns {
			wins = append(wins, window{ns, ne})
		}
	}
	if len(wins) == 0 {
		return nil
	}
	type seg struct{ s, e, ty int }
	var segs []seg
	for i := 0; i < 49 && 0x155+5*i+5 <= sleepRecordSize; i++ {
		q := 0x155 + 5*i
		s := int(binary.LittleEndian.Uint16(raw[q:]))
		e := int(binary.LittleEndian.Uint16(raw[q+2:]))
		ty := int(raw[q+4])
		if s == 0 && e == 0 {
			continue
		}
		if isSleepStage(ty) {
			segs = append(segs, seg{s, e, ty})
		}
	}
	for _, w := range wins {
		var st []SleepStage
		for _, g := range segs {
			if g.s >= w.start && g.s <= w.end {
				st = append(st, SleepStage{
					Start: EpochUTC(base + int64(g.s*60)),
					End:   EpochUTC(base + int64(g.e*60)),
					Type:  g.ty,
				})
			}
		}
		if len(st) == 0 {
			continue
		}
		sec := base + int64(w.start*60)
		out = append(out, SleepRecord{
			DayKey: IstDayKey(sec), StartedAt: EpochUTC(sec), IsNap: true,
			Stages: st, RemMin: sumStageMins(st, 8), LightMin: sumStageMins(st, 4),
			DeepMin: sumStageMins(st, 5), WakeMin: sumStageMins(st, 7),
			TotalMin: sumStageMins(st, 8) + sumStageMins(st, 4) +
				sumStageMins(st, 5) + sumStageMins(st, 7),
		})
	}
	return out
}
