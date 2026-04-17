package tools

import (
	"context"
	"time"
)

// TimeNowTool gives the agent an explicit "what time is it?" handle so
// the LLM can reason about market hours without relying on its own
// (untrustworthy) notion of time. We treat it as metadata, not a fact
// worth citing — the Sources slice is empty on purpose.
type TimeNowTool struct {
	Now func() time.Time
}

// NewTimeNowTool returns a new instance bound to time.Now.
func NewTimeNowTool() *TimeNowTool { return &TimeNowTool{Now: time.Now} }

func (t *TimeNowTool) Name() string { return "time_now" }

func (t *TimeNowTool) Description() string {
	return "Returns the current wall-clock time in UTC and IST, and whether the NSE equity session is currently open."
}

func (t *TimeNowTool) Call(_ context.Context, _ map[string]any) (Result, error) {
	now := t.Now()
	ist := time.FixedZone("IST", 5*3600+30*60)
	return Result{
		Data: map[string]any{
			"utc":         now.UTC().Format(time.RFC3339),
			"ist":         now.In(ist).Format(time.RFC3339),
			"weekday":     now.In(ist).Weekday().String(),
			"market_open": marketOpen(now),
		},
		Summary: "now: " + now.In(ist).Format("02 Jan 15:04 IST"),
		// Intentionally no Sources — this is scaffolding, not ground truth.
	}, nil
}

// marketOpen duplicates portfolio.MarketOpen to avoid a tools → portfolio
// import. Both refer to the same regulation-defined window.
func marketOpen(now time.Time) bool {
	ist := time.FixedZone("IST", 5*3600+30*60)
	t := now.In(ist)
	if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
		return false
	}
	open := time.Date(t.Year(), t.Month(), t.Day(), 9, 15, 0, 0, ist)
	close := time.Date(t.Year(), t.Month(), t.Day(), 15, 30, 0, 0, ist)
	return !t.Before(open) && t.Before(close)
}
