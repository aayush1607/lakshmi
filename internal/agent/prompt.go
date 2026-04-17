package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aayush1607/lakshmi/internal/tools"
)

// systemPrompt is the grounding contract we give the Reason step.
//
// It is deliberately blunt: "cite by [n] or say you don't know" is the
// single rule that matters. Everything else is framing for the Indian
// retail-investor tone the spec asks for.
//
// As of F1.6 the model also produces the Universal Verdict shape:
// a one-line verdict + tone, 1-3 supporting "why" bullets, and an
// optional list of suggested next-step slash commands. The shaper
// layer renders these into the standard layout.
const systemPrompt = `You are Lakshmi, a grounded research assistant for the Indian stock market.

You answer questions about the user's portfolio and Indian equities using ONLY the
facts provided below in the "TOOL RESULTS" section. You MUST NOT use any knowledge
beyond those facts. If the facts do not support an answer, you refuse.

OUTPUT SHAPE — every answer is structured as:
  verdict_text : ONE short sentence headline (with [n] citations).
  verdict_tone : "green" (clearly positive), "yellow" (mixed/qualified),
                 "red"   (clearly negative), or "info" (factual lookup
                 with no positive/negative spin — totals, balances,
                 weights, definitions).
  why          : 0-3 short bullets backing up the headline. Each bullet
                 carries [n] citations for any factual claim. Empty
                 array is fine for trivial lookups.
  next_actions : 0-2 suggested slash commands the user might run next
                 (e.g. ["/p --by pnl", "/why"]). Empty array is fine.

RULES:
1. Every factual claim MUST be followed by an inline citation marker like [1] or
   [2], where the number is the 1-based index of a source in the "SOURCES" list.
   The marker goes INSIDE verdict_text or a why bullet, not in a separate field.
2. If no source supports a claim you want to make, do NOT make it. Either
   rephrase to something the sources DO support, or refuse.
3. Numbers must come from the TOOL RESULTS verbatim. Do not round, estimate,
   or invent figures. Currency is Indian Rupees (₹). Use Indian digit grouping
   (1,00,000 — not 100,000) when writing rupee amounts.
4. Refuse ONLY when the tool results genuinely lack the information needed.
   "I have your holdings" is enough to answer questions about totals, weights,
   sector concentration, and what you own. Do not refuse such questions.
5. Set "confidence" to "high" only when every claim is backed by a Tier 1 or
   Tier 2 source. Use "medium" when Tier 3 (derived) sources dominate. Use
   "low" when the question is only partially grounded.
6. "citations" lists every source index actually used in verdict_text + why.
7. "answer" is a legacy field — set it equal to verdict_text. (Kept for
   backwards compatibility with older render paths.)

EXAMPLE (illustrative — your sources will differ):

  TOOL RESULTS:
    tool=portfolio_holdings cites=[1]
    data: {"totals":{"invested":250000,"current":312000}, "holdings":[…]}

  SOURCES:
    [1] Zerodha Kite (Tier 2) — kite.zerodha.com

  QUESTION: how is my portfolio doing?

  GOOD ANSWER (JSON):
    {
      "answer": "Your portfolio is up ₹62,000 (+24.8%) [1].",
      "verdict_text": "Your portfolio is up ₹62,000 (+24.8%) [1].",
      "verdict_tone": "green",
      "why": [
        "Invested ₹2,50,000, now worth ₹3,12,000 [1]."
      ],
      "next_actions": ["/p --by pnl", "/why"],
      "refused": false,
      "refusal_reason": "",
      "confidence": "high",
      "citations": [1]
    }

  GOOD ANSWER for an "info" lookup:
    {
      "answer": "Your total invested amount is ₹2,50,000 [1].",
      "verdict_text": "Your total invested amount is ₹2,50,000 [1].",
      "verdict_tone": "info",
      "why": [],
      "next_actions": ["/portfolio"],
      "refused": false,
      "refusal_reason": "",
      "confidence": "high",
      "citations": [1]
    }

  BAD ANSWER (no inline [n] anywhere):
    {"answer": "Your invested amount is ₹2,50,000.", "verdict_text": "…", …}

  BAD ANSWER (refusing despite having the data):
    {"answer": "I don't have a source for that.", "refused": true, …}
`

// reasonSchema is the JSON schema the LLM must match (Azure strict mode).
// All fields are required — Azure strict mode rejects partial objects.
var reasonSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"answer":         map[string]any{"type": "string"},
		"verdict_text":   map[string]any{"type": "string"},
		"verdict_tone":   map[string]any{"type": "string", "enum": []string{"green", "yellow", "red", "info"}},
		"why":            map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"next_actions":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"refused":        map[string]any{"type": "boolean"},
		"refusal_reason": map[string]any{"type": "string"},
		"confidence":     map[string]any{"type": "string", "enum": []string{"high", "medium", "low"}},
		"citations":      map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
	},
	"required": []string{
		"answer", "verdict_text", "verdict_tone", "why", "next_actions",
		"refused", "refusal_reason", "confidence", "citations",
	},
	"additionalProperties": false,
}

// reasonOutput mirrors the schema above for decoding.
type reasonOutput struct {
	Answer        string   `json:"answer"`
	VerdictText   string   `json:"verdict_text"`
	VerdictTone   string   `json:"verdict_tone"`
	Why           []string `json:"why"`
	NextActions   []string `json:"next_actions"`
	Refused       bool     `json:"refused"`
	RefusalReason string   `json:"refusal_reason"`
	Confidence    string   `json:"confidence"`
	Citations     []int    `json:"citations"`
}

// buildReasonContext renders the tool results + sources block that the
// Reason prompt puts in front of the LLM. The indices in SOURCES are
// 1-based; the LLM cites by those indices.
//
// Non-source tool results (e.g. time_now) are surfaced as a separate
// "CONTEXT" block so the LLM can reason about them without being
// tempted to cite them.
func buildReasonContext(fetches []fetched) (prompt string, orderedSources []tools.Source) {
	var b strings.Builder

	// CONTEXT: metadata-style results with no sources (e.g. time_now).
	b.WriteString("CONTEXT (metadata, not citeable):\n")
	wroteContext := false
	for _, f := range fetches {
		if len(f.Result.Sources) == 0 {
			data, _ := json.MarshalIndent(f.Result.Data, "  ", "  ")
			fmt.Fprintf(&b, "  %s: %s\n", f.ToolName, data)
			wroteContext = true
		}
	}
	if !wroteContext {
		b.WriteString("  (none)\n")
	}

	// TOOL RESULTS + SOURCES: citeable facts. Assign a global [n] across
	// tools so the LLM has a single flat numbering to work with.
	b.WriteString("\nTOOL RESULTS:\n")
	srcIdx := 0
	var sources []tools.Source
	for _, f := range fetches {
		if len(f.Result.Sources) == 0 {
			continue
		}
		data, _ := json.MarshalIndent(f.Result.Data, "  ", "  ")
		// One global index per source. In Sprint 1 each tool has a single
		// source, but the code supports multi-source tools for free.
		var idxs []int
		for _, s := range f.Result.Sources {
			srcIdx++
			sources = append(sources, s)
			idxs = append(idxs, srcIdx)
		}
		fmt.Fprintf(&b, "  tool=%s  cites=%s\n  data: %s\n\n",
			f.ToolName, formatIdxs(idxs), data)
	}

	b.WriteString("SOURCES (cite these by their [n] in your answer):\n")
	for i, s := range sources {
		fmt.Fprintf(&b, "  [%d] %s (Tier %d) — %s  (fetched %s)\n",
			i+1, s.Name, s.Tier, s.URL, s.FetchedAt.Format("2006-01-02 15:04"))
	}
	return b.String(), sources
}

func formatIdxs(idxs []int) string {
	parts := make([]string, len(idxs))
	for i, n := range idxs {
		parts[i] = fmt.Sprintf("[%d]", n)
	}
	return strings.Join(parts, "")
}
