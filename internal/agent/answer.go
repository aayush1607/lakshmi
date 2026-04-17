// Package agent implements Lakshmi's grounded question-answering loop.
//
// Design invariants (Sprint 1):
//
//   - **Grounding is structural, not advisory.** Every factual claim in
//     an answer must carry a `[n]` citation that maps to a source
//     actually fetched by a tool. The agent validates this after the
//     LLM returns; invalid citations trigger one stricter retry, then
//     a refusal. The LLM never gets to invent sources.
//
//   - **One LLM call per question.** Plan and Fetch are deterministic
//     in Sprint 1: we always call time_now, portfolio_holdings, and
//     sector_lookup. Only the Reason step uses the LLM. This keeps
//     latency predictable and costs bounded.
//
//   - **Refusal is a first-class outcome.** When the question cannot
//     be grounded, the answer field is the refusal reason and
//     `Refused=true`. Callers render a distinctive "I don't have a
//     source for that" message.
//
//   - **Trace is the source of truth for /why.** Every phase appends
//     an Event; the REPL's /why handler just pretty-prints the last
//     Answer's Trace. Nothing else is persisted.
package agent

import "github.com/aayush1607/lakshmi/internal/tools"

// Confidence is a coarse self-assessment emitted by the Reason step.
// The agent does not recompute or override this — the LLM is trusted
// to be honest about its own uncertainty. (A future sprint can make
// this a function of source count, tier, and conflict count.)
type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

// Answer is what Agent.Run returns. Callers should inspect Refused
// before rendering: a refusal has an empty Sources slice and the
// RefusalReason field filled in.
type Answer struct {
	// Text is the assistant's natural-language response. Citations are
	// inlined as bracket markers like "[1]" and "[2]" pointing into
	// Sources by 1-based index.
	//
	// Kept for back-compat with /why and tests that predate F1.6. New
	// renderers should prefer VerdictText + Why for the headline +
	// supporting bullets.
	Text string

	// VerdictText is the one-line headline shown at the top of the
	// shaper layout (F1.6). When empty, callers fall back to Text.
	VerdictText string

	// VerdictTone classifies the headline as positive ("green"),
	// mixed ("yellow"), negative ("red"), or neutral lookup ("info").
	// The shaper uses this to pick the lead glyph and colour.
	VerdictTone string

	// Why is 0-3 short bullets backing up the headline. Each bullet
	// may carry its own [n] citations.
	Why []string

	// NextActions is 0-2 suggested follow-up slash commands the user
	// might want to run after reading this answer.
	NextActions []string

	// Refused is true when the agent declined to answer because the
	// question could not be grounded. Text then holds the refusal
	// explanation and Sources is empty.
	Refused bool

	// RefusalReason is set when Refused is true. It explains *why*
	// grounding failed (e.g. "no tool returned data relevant to the
	// question").
	RefusalReason string

	// Sources are the citeable sources in the order they appear as
	// [1], [2], … in Text. A refusal has an empty slice.
	Sources []tools.Source

	// Confidence is the LLM's self-assessed confidence.
	Confidence Confidence

	// Trace captures every phase for /why. Always populated.
	Trace Trace
}
