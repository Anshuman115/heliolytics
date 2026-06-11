package api

import (
	"net/http"
	"time"
)

func (h *metricsHandler) sleep(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	from, to := dayRange(r)
	rows, err := h.st.ListSleep(r.Context(), from, to)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	type stageRow struct {
		Start string `json:"start"`
		End   string `json:"end"`
		Type  int    `json:"type"`
	}
	type row struct {
		DayKey    string     `json:"dayKey"`
		StartedAt string     `json:"startedAt"`
		Score     int        `json:"score"`
		TotalMins int        `json:"totalMins"`
		DeepMins  int        `json:"deepMins"`
		RemMins   int        `json:"remMins"`
		LightMins int        `json:"lightMins"`
		WakeMins  int        `json:"wakeMins"`
		IsNap     bool       `json:"isNap"`
		Stages    []stageRow `json:"stages,omitempty"`
	}
	out := make([]row, len(rows))
	for i, s := range rows {
		stages := make([]stageRow, len(s.Stages))
		for j, g := range s.Stages {
			stages[j] = stageRow{
				Start: g.Start.Format(time.RFC3339),
				End:   g.End.Format(time.RFC3339),
				Type:  g.Type,
			}
		}
		out[i] = row{
			DayKey: s.DayKey, StartedAt: s.StartedAt.Format(time.RFC3339),
			Score: s.Score, TotalMins: s.TotalMins, DeepMins: s.DeepMins,
			RemMins: s.RemMins, LightMins: s.LightMins, WakeMins: s.WakeMins,
			IsNap: s.IsNap, Stages: stages,
		}
	}
	writeJSON(w, map[string]any{"sleep": out})
}
