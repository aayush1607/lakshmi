package portfolio

import "time"

// MarketOpen reports whether Indian equity markets are currently in the
// regular session (Mon–Fri, 09:15–15:30 IST). Holidays are NOT detected
// — detecting them needs an up-to-date NSE holiday calendar, which is a
// Sprint-2 data source (F2.1). The worst case is that on a holiday we
// label post-close data as "live" until 15:30 IST, which the Kite API's
// own stale LTP will make obvious to the user. Conservative choice.
func MarketOpen(now time.Time) bool {
	ist := time.FixedZone("IST", 5*3600+30*60)
	t := now.In(ist)
	switch t.Weekday() {
	case time.Saturday, time.Sunday:
		return false
	}
	open := time.Date(t.Year(), t.Month(), t.Day(), 9, 15, 0, 0, ist)
	close := time.Date(t.Year(), t.Month(), t.Day(), 15, 30, 0, 0, ist)
	return !t.Before(open) && t.Before(close)
}
