package sync

import (
	"testing"
	"time"
)

func TestNextScheduledTime(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		now    time.Time
		hour   int
		minute int
		want   time.Time
	}{
		{
			name:   "before scheduled time same day",
			now:    time.Date(2026, 7, 4, 1, 30, 0, 0, loc),
			hour:   2,
			minute: 0,
			want:   time.Date(2026, 7, 4, 2, 0, 0, 0, loc),
		},
		{
			name:   "after scheduled time rolls to next day",
			now:    time.Date(2026, 7, 4, 3, 0, 0, 0, loc),
			hour:   2,
			minute: 0,
			want:   time.Date(2026, 7, 5, 2, 0, 0, 0, loc),
		},
		{
			name:   "exactly at scheduled time rolls to next day",
			now:    time.Date(2026, 7, 4, 2, 0, 0, 0, loc),
			hour:   2,
			minute: 0,
			want:   time.Date(2026, 7, 5, 2, 0, 0, 0, loc),
		},
		{
			name:   "custom minute",
			now:    time.Date(2026, 7, 4, 14, 45, 0, 0, loc),
			hour:   14,
			minute: 30,
			want:   time.Date(2026, 7, 5, 14, 30, 0, 0, loc),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nextScheduledTime(tt.now, loc, tt.hour, tt.minute)
			if !got.Equal(tt.want) {
				t.Errorf("nextScheduledTime() = %v, want %v", got, tt.want)
			}
		})
	}
}
