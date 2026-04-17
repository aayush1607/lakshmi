// Package portfolio renders a broker's holdings as a human-readable
// table for the Lakshmi REPL and CLI.
//
// The renderer is deliberately a pure function over a plain slice of
// broker.Holding values: no network calls, no time lookups beyond the
// "as of" timestamp passed in. That makes the output trivially
// golden-testable and keeps the fetch/display concerns separate.
package portfolio

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/aayush1607/lakshmi/internal/broker"
)

// SortBy selects the column the table is sorted by. Weight is the
// default because the top-weighted positions are usually the most
// interesting to see first.
type SortBy int

const (
	SortByWeight SortBy = iota
	SortByPnL
	SortBySymbol
)

// ParseSortBy maps a user-facing flag value ("weight"/"pnl"/"symbol")
// to the SortBy enum. Unknown values return SortByWeight and an error
// the caller can surface.
func ParseSortBy(s string) (SortBy, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "weight", "w":
		return SortByWeight, nil
	case "pnl", "p&l", "p":
		return SortByPnL, nil
	case "symbol", "s", "name":
		return SortBySymbol, nil
	}
	return SortByWeight, fmt.Errorf("unknown sort: %q (want weight|pnl|symbol)", s)
}

// Options bundles the rendering parameters.
type Options struct {
	Sort      SortBy
	AsOf      time.Time     // the moment the data was fetched; shown in the header
	Live      bool          // true during market hours; renders "(live)" vs "(post-close)"
	Width     int           // terminal width; 0 = sensible default
	Colour    bool          // enable ANSI colouring of P&L columns; false for plain
	FromCache bool          // data was served from the local cache
	CacheAge  time.Duration // how old the cached entry is (only meaningful when FromCache)
}

// Totals are the aggregate numbers shown beneath the table.
type Totals struct {
	Invested     float64
	Current      float64
	OverallPnL   float64
	OverallPct   float64
	DayPnL       float64
	DayPct       float64
	HoldingCount int
}

// ComputeTotals is exported so callers (and tests) can reason about the
// aggregate numbers without running the full renderer.
func ComputeTotals(h []broker.Holding) Totals {
	var t Totals
	for _, x := range h {
		qty := float64(x.Quantity)
		invested := qty * x.AvgCost
		current := qty * x.LTP
		prevClose := x.Close
		if prevClose == 0 {
			prevClose = x.LTP // avoid a huge fake day P&L on fresh listings
		}
		t.Invested += invested
		t.Current += current
		t.OverallPnL += current - invested
		t.DayPnL += qty * (x.LTP - prevClose)
		t.HoldingCount++
	}
	if t.Invested > 0 {
		t.OverallPct = 100 * t.OverallPnL / t.Invested
	}
	prevTotal := t.Current - t.DayPnL
	if prevTotal > 0 {
		t.DayPct = 100 * t.DayPnL / prevTotal
	}
	return t
}

// Render returns the full multi-line string to print.
func Render(h []broker.Holding, opts Options) string {
	if len(h) == 0 {
		return renderEmpty(opts)
	}
	totals := ComputeTotals(h)

	// Deep copy before sorting so callers' slice order is preserved.
	rows := make([]broker.Holding, len(h))
	copy(rows, h)
	sortHoldings(rows, opts.Sort, totals.Current)

	var b strings.Builder
	b.WriteString(header(opts))
	b.WriteString("\n\n")
	b.WriteString(renderTable(rows, totals.Current, opts))
	b.WriteString("\n")
	b.WriteString(renderTotals(totals, opts))
	b.WriteString("\n")
	b.WriteString(footer(opts))
	return b.String()
}

func header(opts Options) string {
	ts := opts.AsOf.Format("02 Jan 2006, 15:04 IST")
	tag := "post-close"
	if opts.FromCache {
		tag = "cached · " + ageLabel(opts.CacheAge) + " old"
	} else if opts.Live {
		tag = "live"
	}
	h := lipgloss.NewStyle().Bold(true)
	return h.Render("━━━ HOLDINGS ━━━") + "  (" + tag + " · as of " + ts + ")"
}

func footer(opts Options) string {
	f := lipgloss.NewStyle().Faint(true)
	note := "Source: Zerodha Kite"
	switch {
	case opts.FromCache:
		note += " (cached " + ageLabel(opts.CacheAge) + " ago · run with --fresh to refetch)"
	case opts.Live:
		note += " (live)"
	default:
		note += " (post-close quote)"
	}
	return f.Render(note)
}

// ageLabel formats a duration as a human-friendly "N min", "N h", etc.
func ageLabel(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d h", int(d.Hours()))
	default:
		return fmt.Sprintf("%d d", int(d.Hours()/24))
	}
}

func renderEmpty(opts Options) string {
	return header(opts) + "\n\n  No holdings yet. When you buy stocks on Zerodha, they will appear here.\n"
}

func sortHoldings(h []broker.Holding, by SortBy, totalCurrent float64) {
	switch by {
	case SortBySymbol:
		sort.SliceStable(h, func(i, j int) bool { return h[i].Symbol < h[j].Symbol })
	case SortByPnL:
		sort.SliceStable(h, func(i, j int) bool {
			return pnl(h[i]) > pnl(h[j])
		})
	default: // SortByWeight
		_ = totalCurrent
		sort.SliceStable(h, func(i, j int) bool {
			return float64(h[i].Quantity)*h[i].LTP > float64(h[j].Quantity)*h[j].LTP
		})
	}
}

func pnl(h broker.Holding) float64 {
	return float64(h.Quantity) * (h.LTP - h.AvgCost)
}

// renderTable paints the header row + data rows. Columns are fixed-width
// so rupee and percent values align cleanly across rows.
func renderTable(h []broker.Holding, totalCurrent float64, opts Options) string {
	// Column widths — tuned for a ~100-col terminal. We prefer truncation
	// over wrapping because wrapping an already wide table is unreadable.
	const (
		symW   = 12
		qtyW   = 6
		avgW   = 12
		ltpW   = 12
		pnlW   = 12
		pctW   = 9
		wgtW   = 8
	)

	faint := lipgloss.NewStyle().Faint(true)
	head := lipgloss.NewStyle().Bold(true)

	var b strings.Builder
	b.WriteString(head.Render(fmt.Sprintf(
		" %-*s %*s %*s %*s %*s %*s %*s",
		symW, "SYMBOL",
		qtyW, "QTY",
		avgW, "AVG ₹",
		ltpW, "LTP ₹",
		pnlW, "P&L ₹",
		pctW, "P&L %",
		wgtW, "WEIGHT",
	)))
	b.WriteString("\n")
	b.WriteString(faint.Render(strings.Repeat("─", symW+qtyW+avgW+ltpW+pnlW+pctW+wgtW+8)))
	b.WriteString("\n")

	for _, x := range h {
		invested := float64(x.Quantity) * x.AvgCost
		current := float64(x.Quantity) * x.LTP
		p := current - invested
		pct := 0.0
		if invested > 0 {
			pct = 100 * p / invested
		}
		weight := 0.0
		if totalCurrent > 0 {
			weight = 100 * current / totalCurrent
		}
		pnlStr := signedMoney(p)
		pctStr := signedPct(pct)
		if opts.Colour {
			style := lipgloss.NewStyle()
			switch {
			case p > 0:
				style = style.Foreground(lipgloss.Color("10")) // green
			case p < 0:
				style = style.Foreground(lipgloss.Color("9")) // red
			}
			pnlStr = style.Render(pnlStr)
			pctStr = style.Render(pctStr)
		}
		b.WriteString(fmt.Sprintf(
			" %-*s %*d %*s %*s %*s %*s %*s\n",
			symW, truncate(x.Symbol, symW),
			qtyW, x.Quantity,
			avgW, money(x.AvgCost),
			ltpW, money(x.LTP),
			pnlW, pnlStr,
			pctW, pctStr,
			wgtW, pct1(weight),
		))
	}
	return b.String()
}

func renderTotals(t Totals, opts Options) string {
	head := lipgloss.NewStyle().Bold(true)
	var b strings.Builder
	b.WriteString(head.Render("━━━ TOTALS ━━━"))
	b.WriteString("\n")
	fmt.Fprintf(&b, "  Invested     ₹ %s\n", money(t.Invested))
	fmt.Fprintf(&b, "  Current      ₹ %s\n", money(t.Current))
	fmt.Fprintf(&b, "  Day P&L      ₹ %s (%s)\n", signedMoney(t.DayPnL), signedPct(t.DayPct))
	fmt.Fprintf(&b, "  Overall P&L  ₹ %s (%s)\n", signedMoney(t.OverallPnL), signedPct(t.OverallPct))
	return b.String()
}

// --- number formatting (Indian grouping) ---

// money formats a rupee value with Indian lakh/crore grouping:
//
//	1234567.89 -> "12,34,567.89"
//	99.5        -> "99.50"
func money(v float64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	whole := int64(v)
	frac := int64((v-float64(whole))*100 + 0.5)
	if frac >= 100 {
		whole++
		frac -= 100
	}
	wholeStr := groupIndian(whole)
	out := fmt.Sprintf("%s.%02d", wholeStr, frac)
	if neg {
		out = "-" + out
	}
	return out
}

func signedMoney(v float64) string {
	s := money(v)
	if v > 0 {
		return "+" + s
	}
	return s // money already prefixes '-' for negatives
}

func signedPct(v float64) string {
	if v >= 0 {
		return fmt.Sprintf("+%.2f%%", v)
	}
	return fmt.Sprintf("%.2f%%", v)
}

func pct1(v float64) string { return fmt.Sprintf("%.1f%%", v) }

// groupIndian formats an integer with Indian digit grouping: the last
// three digits are grouped, then pairs beyond that (12,34,56,789).
func groupIndian(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	last3 := s[len(s)-3:]
	rest := s[:len(s)-3]
	// Group `rest` in pairs from the right.
	var chunks []string
	for len(rest) > 2 {
		chunks = append([]string{rest[len(rest)-2:]}, chunks...)
		rest = rest[:len(rest)-2]
	}
	if rest != "" {
		chunks = append([]string{rest}, chunks...)
	}
	return strings.Join(chunks, ",") + "," + last3
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}
