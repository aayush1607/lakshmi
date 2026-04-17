// Package shaper — confidence scoring.
//
// Confidence is a deterministic 0..100 score derived from the source
// mix of an Answer. The model self-rates, but we don't trust it: a
// model that hallucinates is the same model that overrates its own
// certainty. Computing confidence from observable facts (source count,
// tier weighting, recency) is harder to game.
//
// Formula (intentionally simple and tunable):
//
//	tier weight: T1=1.0, T2=0.8, T3=0.5, other=0.3
//	base       = average tier weight × 100
//	count bonus= +5 per source above 1, capped at +15
//	stale pen  = -10 if any source is older than 24h
//	floor      = 10  (a sourced answer is never "0% confident")
//	ceiling    = 99  (we don't claim 100%)
//
// Refusals report 0 — there's nothing to be confident about.
package shaper

import "time"

// ComputeConfidence returns the deterministic 0..100 score for a set
// of sources, given the wall clock used to judge recency. `now` lets
// callers (and tests) inject a fixed clock.
func ComputeConfidence(sources []Source, now time.Time) int {
	if len(sources) == 0 {
		return 0
	}
	var sum float64
	stale := false
	for _, s := range sources {
		sum += tierWeight(s.Tier)
		if !s.FetchedAt.IsZero() && now.Sub(s.FetchedAt) > 24*time.Hour {
			stale = true
		}
	}
	avg := sum / float64(len(sources))
	score := avg * 100

	bonus := 5 * (len(sources) - 1)
	if bonus > 15 {
		bonus = 15
	}
	score += float64(bonus)

	if stale {
		score -= 10
	}

	if score < 10 {
		score = 10
	}
	if score > 99 {
		score = 99
	}
	return int(score)
}

func tierWeight(tier int) float64 {
	switch tier {
	case 1:
		return 1.0
	case 2:
		return 0.8
	case 3:
		return 0.5
	default:
		return 0.3
	}
}
