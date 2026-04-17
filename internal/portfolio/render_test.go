package portfolio

import (
	"strings"
	"testing"
	"time"

	"github.com/aayush1607/lakshmi/internal/broker"
)

func sampleHoldings() []broker.Holding {
	return []broker.Holding{
		{Symbol: "TCS", Quantity: 15, AvgCost: 3420, LTP: 3980, Close: 3950},
		{Symbol: "INFY", Quantity: 30, AvgCost: 1420, LTP: 1380, Close: 1390},
		{Symbol: "HDFCBANK", Quantity: 42, AvgCost: 1510, LTP: 1642, Close: 1640},
	}
}

func TestComputeTotals(t *testing.T) {
	h := sampleHoldings()
	got := ComputeTotals(h)
	// Invested: 15*3420 + 30*1420 + 42*1510 = 51300 + 42600 + 63420 = 157320
	if got.Invested != 157320 {
		t.Errorf("Invested = %v, want 157320", got.Invested)
	}
	// Current: 15*3980 + 30*1380 + 42*1642 = 59700 + 41400 + 68964 = 170064
	if got.Current != 170064 {
		t.Errorf("Current = %v, want 170064", got.Current)
	}
	// Overall P&L = 170064 - 157320 = 12744
	if got.OverallPnL != 12744 {
		t.Errorf("OverallPnL = %v, want 12744", got.OverallPnL)
	}
	if got.HoldingCount != 3 {
		t.Errorf("HoldingCount = %d, want 3", got.HoldingCount)
	}
	// Overall % = 12744 / 157320 * 100 ≈ 8.10
	if got.OverallPct < 8.09 || got.OverallPct > 8.11 {
		t.Errorf("OverallPct ≈ %v, want ≈ 8.10", got.OverallPct)
	}
}

func TestSortByWeight(t *testing.T) {
	h := sampleHoldings()
	sortHoldings(h, SortByWeight, 0)
	// HDFCBANK: 42*1642=68964; TCS: 15*3980=59700; INFY: 30*1380=41400.
	want := []string{"HDFCBANK", "TCS", "INFY"}
	for i, sym := range want {
		if h[i].Symbol != sym {
			t.Fatalf("weight sort[%d] = %s, want %s (%v)", i, h[i].Symbol, sym, names(h))
		}
	}
}

func TestSortByPnL(t *testing.T) {
	h := sampleHoldings()
	sortHoldings(h, SortByPnL, 0)
	// TCS: 15*(3980-3420)=8400; HDFCBANK: 42*(1642-1510)=5544; INFY: 30*(1380-1420)=-1200.
	want := []string{"TCS", "HDFCBANK", "INFY"}
	for i, sym := range want {
		if h[i].Symbol != sym {
			t.Fatalf("pnl sort[%d] = %s, want %s (%v)", i, h[i].Symbol, sym, names(h))
		}
	}
}

func TestSortBySymbol(t *testing.T) {
	h := sampleHoldings()
	sortHoldings(h, SortBySymbol, 0)
	want := []string{"HDFCBANK", "INFY", "TCS"}
	for i, sym := range want {
		if h[i].Symbol != sym {
			t.Fatalf("symbol sort[%d] = %s, want %s (%v)", i, h[i].Symbol, sym, names(h))
		}
	}
}

func TestParseSortBy(t *testing.T) {
	cases := map[string]SortBy{
		"":       SortByWeight,
		"weight": SortByWeight,
		"w":      SortByWeight,
		"pnl":    SortByPnL,
		"P":      SortByPnL,
		"symbol": SortBySymbol,
	}
	for in, want := range cases {
		got, err := ParseSortBy(in)
		if err != nil || got != want {
			t.Errorf("ParseSortBy(%q) = (%v,%v), want (%v,nil)", in, got, err, want)
		}
	}
	if _, err := ParseSortBy("garbage"); err == nil {
		t.Error("ParseSortBy(garbage): want error")
	}
}

func TestRenderNonEmpty(t *testing.T) {
	out := Render(sampleHoldings(), Options{
		Sort: SortByWeight,
		AsOf: time.Date(2026, 4, 17, 10, 42, 0, 0, time.FixedZone("IST", 5*3600+30*60)),
		Live: true,
	})
	// Sanity assertions — keep loose so we can evolve styling without
	// constantly updating tests. Golden tests can arrive later.
	for _, want := range []string{"HOLDINGS", "TOTALS", "TCS", "INFY", "HDFCBANK", "live", "17 Apr 2026"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n---\n%s", want, out)
		}
	}
	// P&L totals should show a plus sign.
	if !strings.Contains(out, "+12,744") {
		t.Errorf("expected formatted total P&L +12,744 in output:\n%s", out)
	}
}

func TestRenderEmpty(t *testing.T) {
	out := Render(nil, Options{Live: false, AsOf: time.Now()})
	if !strings.Contains(out, "No holdings yet") {
		t.Errorf("empty state missing: %s", out)
	}
	if strings.Contains(out, "TOTALS") {
		t.Errorf("empty state should not render TOTALS block: %s", out)
	}
}

func TestRenderPostClose(t *testing.T) {
	out := Render(sampleHoldings(), Options{Live: false, AsOf: time.Now()})
	if !strings.Contains(out, "post-close") {
		t.Errorf("expected post-close note in header/footer: %s", out)
	}
}

func TestGroupIndianAndMoney(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0.00"},
		{99.5, "99.50"},
		{1234.5, "1,234.50"},
		{123456.789, "1,23,456.79"},
		{12345678.9, "1,23,45,678.90"},
		{-1234.5, "-1,234.50"},
	}
	for _, c := range cases {
		if got := money(c.in); got != c.want {
			t.Errorf("money(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMarketOpen(t *testing.T) {
	ist := time.FixedZone("IST", 5*3600+30*60)
	cases := []struct {
		name string
		t    time.Time
		want bool
	}{
		{"sat", time.Date(2026, 4, 18, 11, 0, 0, 0, ist), false},
		{"sun", time.Date(2026, 4, 19, 11, 0, 0, 0, ist), false},
		{"fri 10:00", time.Date(2026, 4, 17, 10, 0, 0, 0, ist), true},
		{"fri 09:14", time.Date(2026, 4, 17, 9, 14, 0, 0, ist), false},
		{"fri 09:15", time.Date(2026, 4, 17, 9, 15, 0, 0, ist), true},
		{"fri 15:29", time.Date(2026, 4, 17, 15, 29, 0, 0, ist), true},
		{"fri 15:30", time.Date(2026, 4, 17, 15, 30, 0, 0, ist), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := MarketOpen(c.t); got != c.want {
				t.Fatalf("MarketOpen(%s) = %v, want %v", c.name, got, c.want)
			}
		})
	}
}

func names(h []broker.Holding) []string {
	out := make([]string, len(h))
	for i, x := range h {
		out[i] = x.Symbol
	}
	return out
}
