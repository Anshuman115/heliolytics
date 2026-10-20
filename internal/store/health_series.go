package store

import (
	"context"

	"github.com/Anshuman115/heliolytics/internal/store/db"
)

type SeriesDay struct {
	Metric string `json:"metric"`
	CompactDay
}

func (s *Store) ListHealthSamplesCompact(ctx context.Context, from, to string) ([]SeriesDay, error) {
	fromAt, toAt, err := dayRange(from, to)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListSeriesCompact(ctx, db.ListSeriesCompactParams{
		FromAt: tsAt(fromAt),
		ToAt:   tsAt(toAt),
	})
	if err != nil {
		return nil, err
	}
	out := make([]SeriesDay, 0, len(rows))
	for _, r := range rows {
		out = append(out, SeriesDay{
			Metric: r.Metric,
			CompactDay: CompactDay{
				DayKey:    r.DayKey.Time.Format(dateLayout),
				StartTime: r.StartTime.Time,
				Offsets:   r.Offsets,
				Values:    r.Values,
			},
		})
	}
	return out, nil
}
