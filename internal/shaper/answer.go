// Package shaper renders every Lakshmi answer in one consistent shape
// (F1.6 "Universal Verdict Format"):
//
//	━━━ VERDICT ━━━
//	🟢 / 🟡 / 🔴  one-line call
//
//	━━━ WHY ━━━
//	• bullet 1
//	• bullet 2
//
//	━━━ DETAIL ━━━
//	(numbers, tables, prose — collapsed when long)
//
//	━━━ SOURCES ━━━
//	[1] source — what it provided · tier
//
//	━━━ CONFIDENCE: NN% ━━━
//	Based on N sources · recency: <age>
//
//	📎 Next: <command>  ·  <command>
//
// The shaper is **pure**: a function from Answer to string. No I/O, no
// mutation, no global state. This is what makes the same question
// asked twice produce identical layout (F1.6 acceptance #5) and what
// lets us golden-test the renderer aggressively.
//
// Callers (agent, /portfolio, /help, refusals) build an Answer, hand
// it to Render, and print the result. The shape is enforced at the
// type level: there is no "freeform output" code path.
package shaper

import "time"

// Source is the citeable evidence backing an Answer. It mirrors
// tools.Source structurally but lives here so the shaper has zero
// dependencies on the tools / agent layers — that's what lets every
// producer (agent, /portfolio, REPL chrome) speak the same shape
// without forcing those packages into a cycle.
type Source struct {
	Name      string
	URL       string
	Tier      int // 1=official, 2=broker, 3=derived
	FetchedAt time.Time
}

// Verdict colour. Drives the lead emoji/icon and the verdict line tint.
type Verdict string

const (
	VerdictGreen  Verdict = "green"  // clear positive / safe / do-it
	VerdictYellow Verdict = "yellow" // mixed / lean one way / conditions apply
	VerdictRed    Verdict = "red"    // clear negative / avoid / don't do it
	VerdictNone   Verdict = ""       // informational answer — no verdict slot
)

// Kind disambiguates layout variants. Most answers are Normal; the
// others suppress sections (e.g. UnknownCommand has no Sources).
type Kind string

const (
	KindNormal        Kind = "normal"
	KindInformational Kind = "info"    // pure Q&A, verdict slot replaced by an answer line
	KindRefusal       Kind = "refusal" // grounded refusal — no verdict, sources optional
	KindUnknown       Kind = "unknown" // unknown command — only Verdict + Next
	KindError         Kind = "error"   // tool/network failure — Verdict red + Why + Next
)

// Answer is the typed structure every command produces. Render turns
// this into the final on-screen layout.
//
// Empty fields are dropped from the rendered output (e.g. no Why
// bullets → no "WHY" section). This lets simple commands omit cargo
// without violating the "always the same shape" guarantee.
type Answer struct {
	Kind Kind

	// Verdict renders the top section. For KindInformational this slot
	// holds the one-line answer instead of a colour-coded call.
	Verdict     Verdict
	VerdictText string

	// Why is 1-N short bullets backing up the Verdict. Each bullet
	// SHOULD reference a Sources entry by [n]; the renderer just
	// formats them — grounding is enforced upstream by the agent.
	Why []string

	// Detail is the long-form payload (tables, dumps, prose). The
	// renderer collapses it past detailCollapseLines and surfaces a
	// "type 'more' to expand" hint. The REPL's /more handler is what
	// actually expands it in the next render call.
	Detail string

	// Sources are the citeable evidence. The shaper defines its own
	// Source type to avoid coupling to the agent/tools packages.
	Sources []Source

	// Confidence is 0..100. Computed deterministically from the source
	// mix; see ComputeConfidence. Set to -1 to suppress the section.
	Confidence int

	// NextActions are runnable slash-commands the user might want next
	// (e.g. "/why", "/portfolio --by pnl"). Empty means no Next line.
	NextActions []string

	// AsOf is the wall-clock when the underlying data was fetched. Used
	// to render the "recency" hint in the Confidence line.
	AsOf time.Time
}
