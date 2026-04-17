// Package agent — render: thin adapter from agent.Answer (the typed
// result of Run) to shaper.Answer (the universal display shape).
//
// This file owns the *translation*, not the *rendering*. All the
// layout work — sectioning, glyphs, collapse logic, colours — lives
// in internal/shaper. Keeping it that way means /portfolio,
// /unknown-command, and any future producer of an Answer all share
// the same visual grammar (the F1.6 promise).
package agent

import (
	"time"

	"github.com/aayush1607/lakshmi/internal/shaper"
	"github.com/aayush1607/lakshmi/internal/tools"
)

// Render turns an Answer into the printable string the REPL emits.
// It uses the default theme; tests and the (planned) --no-color flag
// can call shaper.Render directly with PlainTheme.
func Render(a Answer) string {
	return shaper.Render(ToShaper(a), shaper.DefaultTheme(), false)
}

// RenderExpanded is the same as Render but forces the Detail block to
// be shown in full. Wired to the REPL's `more` command.
func RenderExpanded(a Answer) string {
	return shaper.Render(ToShaper(a), shaper.DefaultTheme(), true)
}

// ToShaper translates an agent.Answer into the universal Answer shape.
// Confidence is recomputed deterministically from the source mix
// (we don't blindly trust the LLM's self-rating). Exported so callers
// that want to compose answers (e.g. /portfolio) can also speak the
// shaper vocabulary directly.
func ToShaper(a Answer) shaper.Answer {
	out := shaper.Answer{
		VerdictText: firstNonEmpty(a.VerdictText, a.Text),
		Why:         a.Why,
		Sources:     toShaperSources(a.Sources),
		NextActions: a.NextActions,
		AsOf:        latestFetched(a.Sources),
	}

	switch {
	case a.Refused:
		out.Kind = shaper.KindRefusal
		out.Verdict = shaper.VerdictRed
		if a.RefusalReason != "" && len(out.Why) == 0 {
			// Surface the refusal reason as a single bullet so the user
			// can see WHY we refused, not just THAT we refused.
			out.Why = []string{a.RefusalReason}
		}
	default:
		out.Kind = shaper.KindInformational
		out.Verdict = toneToVerdict(a.VerdictTone)
		out.Confidence = shaper.ComputeConfidence(out.Sources, time.Now())
	}
	return out
}

// toShaperSources copies tools.Source values into the structurally
// identical shaper.Source. The duplication exists to keep the shaper
// package free of any agent/tools dependency (which would otherwise
// cycle: tools → portfolio → shaper → tools).
func toShaperSources(in []tools.Source) []shaper.Source {
	out := make([]shaper.Source, len(in))
	for i, s := range in {
		out[i] = shaper.Source{
			Name:      s.Name,
			URL:       s.URL,
			Tier:      s.Tier,
			FetchedAt: s.FetchedAt,
		}
	}
	return out
}

// toneToVerdict maps the LLM-provided tone string into the shaper enum.
func toneToVerdict(tone string) shaper.Verdict {
	switch tone {
	case "green":
		return shaper.VerdictGreen
	case "yellow":
		return shaper.VerdictYellow
	case "red":
		return shaper.VerdictRed
	default:
		// "info" or empty → no coloured verdict, just the headline.
		return shaper.VerdictNone
	}
}

// latestFetched returns the most recent FetchedAt across sources, used
// to render the "as of" line. Zero if no source carries a timestamp.
func latestFetched(sources []tools.Source) time.Time {
	var latest time.Time
	for _, s := range sources {
		if s.FetchedAt.After(latest) {
			latest = s.FetchedAt
		}
	}
	return latest
}
