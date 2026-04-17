// Package portfolio — shape: build a shaper.Answer for the holdings
// view. Keeps the rich Render() table as the Detail block and produces
// a one-line P&L verdict so /portfolio reads in the same Verdict→Why→
// Detail→Sources rhythm as every other Lakshmi answer (F1.6).
package portfolio

import (
	"fmt"
	"time"

	"github.com/aayush1607/lakshmi/internal/broker"
	"github.com/aayush1607/lakshmi/internal/shaper"
)

// Shape returns a shaper.Answer for a holdings snapshot. The Detail
// section embeds the existing Render() table verbatim — collapsing
// kicks in at 20 lines, which is roughly 12 holdings.
//
// Verdict tone is driven entirely by overall P&L: green when up, red
// when down, info on a flat / no-holdings portfolio. We don't try to
// be clever about thresholds (e.g. "down 0.2% is basically flat") —
// users can read the number; the colour is a quick scan signal, not a
// recommendation.
func Shape(h []broker.Holding, opts Options) shaper.Answer {
	now := opts.AsOf
	if now.IsZero() {
		now = time.Now()
	}

	src := shaper.Source{
		Name:      "Zerodha Kite",
		URL:       "kite.zerodha.com",
		Tier:      2,
		FetchedAt: now,
	}

	// Empty portfolio: short-circuit with a friendly informational
	// answer. No verdict colour — there's nothing to grade.
	if len(h) == 0 {
		return shaper.Answer{
			Kind:        shaper.KindInformational,
			Verdict:     shaper.VerdictNone,
			VerdictText: "No holdings yet — your Zerodha account is empty.",
			Sources:     []shaper.Source{src},
			Confidence:  shaper.ComputeConfidence([]shaper.Source{src}, now),
			NextActions: []string{"/login"},
			AsOf:        now,
		}
	}

	totals := ComputeTotals(h)
	verdict := shaper.VerdictNone
	tone := "info"
	switch {
	case totals.OverallPnL > 0:
		verdict, tone = shaper.VerdictGreen, "up"
	case totals.OverallPnL < 0:
		verdict, tone = shaper.VerdictRed, "down"
	}
	_ = tone // tone variable is just here for future use; verdict drives colour.

	headline := fmt.Sprintf("Portfolio is %s ₹%s (%+.1f%%) — %d holding%s, ₹%s invested → ₹%s now.",
		signWord(totals.OverallPnL),
		formatINR(absFloat(totals.OverallPnL)),
		totals.OverallPct,
		totals.HoldingCount, plural(totals.HoldingCount),
		formatINR(totals.Invested),
		formatINR(totals.Current),
	)

	why := buildWhy(h, totals)

	return shaper.Answer{
		Kind:        shaper.KindInformational,
		Verdict:     verdict,
		VerdictText: headline,
		Why:         why,
		Detail:      Render(h, opts),
		Sources:     []shaper.Source{src},
		Confidence:  shaper.ComputeConfidence([]shaper.Source{src}, now),
		NextActions: nextActions(h, opts.Sort),
		AsOf:        now,
	}
}

// nextActions builds the suggested follow-up commands. Crucially every
// suggestion must be a real, runnable command — placeholders like
// "/ask why is X up/down" send the user on a goose chase. We pick the
// alternate sort that's most useful relative to what they're already
// looking at, and (when there's a clear loser) suggest a concrete
// "/ask why is <SYMBOL> down" question.
func nextActions(h []broker.Holding, current SortBy) []string {
	var out []string
	// Suggest the *other* useful sort — never the one already in use.
	switch current {
	case SortByPnL:
		out = append(out, "/p --by weight")
	case SortBySymbol:
		out = append(out, "/p --by pnl")
	default:
		out = append(out, "/p --by pnl")
	}
	// If something is meaningfully down, offer a concrete why-is question
	// rather than a templated placeholder. Skip when nothing is in the red.
	if _, worst := topMovers(h); worst != nil && pnl(*worst) < 0 {
		out = append(out, fmt.Sprintf("/ask why is %s down", worst.Symbol))
	}
	return out
}

// buildWhy returns 1-3 short bullets that justify the verdict: today's
// P&L line, top winner, and top loser. Only included when there's
// signal to report (skip the winner bullet on a portfolio that's
// uniformly down, etc.).
func buildWhy(h []broker.Holding, totals Totals) []string {
	var out []string
	if totals.DayPnL != 0 {
		out = append(out, fmt.Sprintf("Today: %s ₹%s (%+.2f%%).",
			signWord(totals.DayPnL),
			formatINR(absFloat(totals.DayPnL)),
			totals.DayPct,
		))
	}
	best, worst := topMovers(h)
	if best != nil && pnl(*best) > 0 {
		out = append(out, fmt.Sprintf("Top winner: %s up ₹%s (%+.1f%%).",
			best.Symbol,
			formatINR(pnl(*best)),
			pctChange(*best),
		))
	}
	if worst != nil && pnl(*worst) < 0 {
		out = append(out, fmt.Sprintf("Top loser: %s down ₹%s (%+.1f%%).",
			worst.Symbol,
			formatINR(absFloat(pnl(*worst))),
			pctChange(*worst),
		))
	}
	return out
}

// topMovers returns pointers to the holding with the largest absolute
// gain and the largest absolute loss. Either may be nil if no holding
// has moved in that direction.
func topMovers(h []broker.Holding) (best, worst *broker.Holding) {
	for i := range h {
		p := pnl(h[i])
		if best == nil || p > pnl(*best) {
			best = &h[i]
		}
		if worst == nil || p < pnl(*worst) {
			worst = &h[i]
		}
	}
	return best, worst
}

// pctChange returns (LTP - AvgCost) / AvgCost * 100. Returns 0 on
// zero-cost holdings (free shares from corporate actions, in theory).
func pctChange(h broker.Holding) float64 {
	if h.AvgCost == 0 {
		return 0
	}
	return 100 * (h.LTP - h.AvgCost) / h.AvgCost
}

// signWord returns "up" / "down" / "flat" for use in the verdict line.
func signWord(v float64) string {
	switch {
	case v > 0:
		return "up"
	case v < 0:
		return "down"
	default:
		return "flat at"
	}
}

// formatINR applies Indian digit grouping (1,00,000) to a rupee amount.
// Rounds to whole rupees — fractional paise add visual noise without
// changing any decision the user might make.
func formatINR(v float64) string {
	if v < 0 {
		v = -v
	}
	n := int64(v + 0.5)
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	// Last three digits, then groups of two.
	last3 := n % 1000
	rest := n / 1000
	out := fmt.Sprintf("%03d", last3)
	for rest > 0 {
		group := rest % 100
		rest /= 100
		if rest > 0 {
			out = fmt.Sprintf("%02d,", group) + out
		} else {
			out = fmt.Sprintf("%d,", group) + out
		}
	}
	return out
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
