package metrics

import "time"

// WeekProjection is spend extrapolated to the end of a Monday-09:00 week.
type WeekProjection struct {
	Cost  float64   `json:"cost"`
	Until time.Time `json:"until"`
}

// minProjectionElapsed keeps Monday morning from extrapolating a week out of
// a few hours of work.
const minProjectionElapsed = 24 * time.Hour

// ProjectWeekCost extrapolates cost linearly over the elapsed part of the
// week: cost x (7 days / elapsed). It applies only to the dashboard's default
// window, which starts on a Monday at 09:00 local time, and only once a full
// day of that week has passed; ok is false otherwise.
func ProjectWeekCost(lookbackFrom, now time.Time, cost float64) (WeekProjection, bool) {
	local := lookbackFrom.In(now.Location())
	if local.Weekday() != time.Monday || local.Hour() != 9 || local.Minute() != 0 || local.Second() != 0 {
		return WeekProjection{}, false
	}
	week := 7 * 24 * time.Hour
	until := lookbackFrom.Add(week)
	elapsed := now.Sub(lookbackFrom)
	if elapsed < minProjectionElapsed || !now.Before(until) {
		return WeekProjection{}, false
	}
	return WeekProjection{Cost: cost * float64(week) / float64(elapsed), Until: until}, true
}
