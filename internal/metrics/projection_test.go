package metrics

import (
	"math"
	"testing"
	"time"
)

func TestProjectWeekCost(t *testing.T) {
	monday := time.Date(2026, 10, 5, 9, 0, 0, 0, time.Local)
	cases := []struct {
		name     string
		from     time.Time
		now      time.Time
		cost     float64
		wantOK   bool
		wantCost float64
	}{
		{"two days in", monday, monday.Add(48 * time.Hour), 200, true, 700},
		{"under a day", monday, monday.Add(23 * time.Hour), 50, false, 0},
		{"week over", monday, monday.Add(7 * 24 * time.Hour), 900, false, 0},
		{"not a Monday window", monday.Add(24 * time.Hour), monday.Add(72 * time.Hour), 100, false, 0},
		{"Monday but not 09:00", monday.Add(time.Hour), monday.Add(48 * time.Hour), 100, false, 0},
	}
	for _, tc := range cases {
		got, ok := ProjectWeekCost(tc.from, tc.now, tc.cost)
		if ok != tc.wantOK {
			t.Errorf("%s: ok = %v, want %v", tc.name, ok, tc.wantOK)
			continue
		}
		if ok && math.Abs(got.Cost-tc.wantCost) > 1e-9 {
			t.Errorf("%s: cost = %.4f, want %.4f", tc.name, got.Cost, tc.wantCost)
		}
		if ok && !got.Until.Equal(monday.Add(7*24*time.Hour)) {
			t.Errorf("%s: until = %v, want next Monday 09:00", tc.name, got.Until)
		}
	}
}
