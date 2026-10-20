package api

import (
	"net/http"
	"sort"

	"github.com/Anshuman115/heliolytics/internal/middleware"
	"github.com/Anshuman115/heliolytics/internal/store"
)

// sleepStage is one stored stage boundary; end is exclusive.
type sleepStage struct {
	Start string `json:"start"`
	End   string `json:"end"`
	Type  int    `json:"type"`
}

type sleepSession struct {
	StartedAt       string       `json:"startedAt"`
	EndedAt         string       `json:"endedAt"`
	Score           *int         `json:"score"`
	TotalMins       int          `json:"totalMins"`
	DeepMins        int          `json:"deepMins"`
	RemMins         int          `json:"remMins"`
	LightMins       int          `json:"lightMins"`
	WakeMins        int          `json:"wakeMins"`
	StagesTruncated bool         `json:"stagesTruncated"`
	Stages          []sleepStage `json:"stages"`
}

type sleepDay struct {
	DayKey string         `json:"dayKey"`
	Main   *sleepSession  `json:"main"`
	Naps   []sleepSession `json:"naps"`
}

type sleepResponse struct {
	Days []sleepDay `json:"days"`
}

func (h *metricsHandler) sleep(w http.ResponseWriter, r *http.Request) {
	from, to, failure := h.rangeBounds(r, maxDailyRangeDays)
	if failure != nil {
		middleware.WriteError(w, failure)
		return
	}
	rows, err := h.st.ListSleep(r.Context(), from, to)
	if err != nil {
		writeStoreFailure(w, "sleep", err)
		return
	}
	byDay := make(map[string][]store.SleepMetric, len(rows))
	days := make([]string, 0, len(rows))
	for _, row := range rows {
		if _, seen := byDay[row.DayKey]; !seen {
			days = append(days, row.DayKey)
		}
		byDay[row.DayKey] = append(byDay[row.DayKey], row)
	}
	sort.Strings(days)
	out := make([]sleepDay, 0, len(days))
	for _, day := range days {
		out = append(out, groupSleepDay(day, byDay[day]))
	}
	writeJSON(w, http.StatusOK, sleepResponse{Days: out})
}

func groupSleepDay(dayKey string, metrics []store.SleepMetric) sleepDay {
	naps := make([]store.SleepRow, 0, len(metrics))
	var main *store.SleepRow
	for i := range metrics {
		row := metrics[i].Row
		if row.IsNap {
			naps = append(naps, row)
			continue
		}
		if main == nil || row.TotalMins > main.TotalMins ||
			(row.TotalMins == main.TotalMins && row.StartedAt.Before(main.StartedAt)) {
			main = &row
		}
	}
	sort.Slice(naps, func(i, j int) bool { return naps[i].StartedAt.Before(naps[j].StartedAt) })

	out := sleepDay{DayKey: dayKey, Naps: make([]sleepSession, 0, len(naps))}
	if main != nil {
		session := sleepSessionOf(*main, false)
		out.Main = &session
	}
	for _, nap := range naps {
		out.Naps = append(out.Naps, sleepSessionOf(nap, true))
	}
	return out
}

func sleepSessionOf(row store.SleepRow, isNap bool) sleepSession {
	stages := make([]sleepStage, 0, len(row.Stages))
	for _, stage := range row.Stages {
		stages = append(stages, sleepStage{
			Start: formatInstant(stage.Start),
			End:   formatInstant(stage.End),
			Type:  stage.Type,
		})
	}
	session := sleepSession{
		StartedAt:       formatInstant(row.StartedAt),
		EndedAt:         formatInstant(row.EndedAt),
		Score:           row.Score,
		TotalMins:       row.TotalMins,
		DeepMins:        row.DeepMins,
		RemMins:         row.RemMins,
		LightMins:       row.LightMins,
		WakeMins:        row.WakeMins,
		StagesTruncated: row.StagesTruncated,
		Stages:          stages,
	}
	if isNap {
		session.Score = nil
	}
	return session
}
