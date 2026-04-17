package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aayush1607/lakshmi/internal/llm"
	"github.com/aayush1607/lakshmi/internal/tools"
)

// Agent is a stateless orchestrator: one instance per process, many
// concurrent Run calls are safe (assuming the wrapped Client and Tools
// are safe). Conversation history is caller-owned and passed in.
type Agent struct {
	LLM   llm.Client
	Tools *tools.Registry
	Now   func() time.Time

	// Plan lists the tool names invoked for every question in Sprint 1.
	// Kept as a field (rather than hard-coded in Run) so tests can swap
	// in a smaller plan and CLI subcommands can reuse the same Agent
	// with a reduced toolset.
	Plan []string
}

// Options bundles optional constructor args. Defaults fill in sensible
// values — see New.
type Options struct {
	LLM   llm.Client
	Tools *tools.Registry
	Now   func() time.Time
	Plan  []string
}

// New constructs an Agent. The default Plan is the Sprint 1 trio:
// time_now → portfolio_holdings → sector_lookup.
func New(opts Options) *Agent {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if len(opts.Plan) == 0 {
		opts.Plan = []string{"time_now", "portfolio_holdings", "sector_lookup"}
	}
	return &Agent{LLM: opts.LLM, Tools: opts.Tools, Now: opts.Now, Plan: opts.Plan}
}

// fetched bundles a tool call's output with its name so buildReasonContext
// can label things in the prompt.
type fetched struct {
	ToolName string
	Result   tools.Result
	Err      error
	Took     time.Duration
}

// Run executes one full pipeline for a single question.
//
// history is the prior user/assistant turns in THIS session. The agent
// prepends them (not the tool results) to the Reason call so follow-up
// questions like "what about energy?" inherit the context of the prior
// "what's my IT exposure?".
func (a *Agent) Run(ctx context.Context, query string, history []llm.Message) (Answer, error) {
	start := a.Now()
	trace := Trace{Query: query}

	// -- Phase 1: Plan (deterministic). ---------------------------------
	planStart := a.Now()
	plan := make([]tools.Tool, 0, len(a.Plan))
	var missing []string
	for _, name := range a.Plan {
		if t, ok := a.Tools.Get(name); ok {
			plan = append(plan, t)
		} else {
			missing = append(missing, name)
		}
	}
	trace.record(Event{
		Phase:    PhasePlan,
		Note:     fmt.Sprintf("tools=%s", strings.Join(a.Plan, ",")),
		Duration: a.Now().Sub(planStart),
	})
	if len(plan) == 0 {
		trace.Total = a.Now().Sub(start)
		return Answer{
			Refused:       true,
			Text:          "I can't answer — no tools are wired up.",
			RefusalReason: "empty plan: " + strings.Join(missing, ","),
			Trace:         trace,
		}, nil
	}

	// -- Phase 2: Fetch (parallel). -------------------------------------
	fetches := a.fetchAll(ctx, plan, &trace)

	// -- Phase 3: Reason (single LLM call, with one retry on bad cite). -
	promptCtx, orderedSources := buildReasonContext(fetches)
	out, reasonErr := a.reasonOnce(ctx, query, promptCtx, history, &trace, false)
	if reasonErr != nil {
		trace.Total = a.Now().Sub(start)
		return Answer{}, reasonErr
	}

	// -- Phase 4: Shape (validate grounding; retry once if broken). -----
	ans, retry := a.shape(out, orderedSources, &trace)
	if retry {
		out2, err2 := a.reasonOnce(ctx, query, promptCtx, history, &trace, true)
		if err2 == nil {
			ans, _ = a.shape(out2, orderedSources, &trace)
		}
	}
	ans.Trace = trace
	ans.Trace.Total = a.Now().Sub(start)
	return ans, nil
}

// fetchAll runs every planned tool in parallel and returns results in
// the same order as the plan (so prompt rendering is deterministic).
func (a *Agent) fetchAll(ctx context.Context, plan []tools.Tool, trace *Trace) []fetched {
	out := make([]fetched, len(plan))
	var wg sync.WaitGroup
	for i, t := range plan {
		wg.Add(1)
		go func(i int, t tools.Tool) {
			defer wg.Done()
			start := a.Now()
			// Sprint 1 tools take no args. Later phases will pass the
			// Plan-step-selected args through here.
			r, err := t.Call(ctx, nil)
			out[i] = fetched{ToolName: t.Name(), Result: r, Err: err, Took: a.Now().Sub(start)}
		}(i, t)
	}
	wg.Wait()
	for _, f := range out {
		ev := Event{
			Phase:    PhaseFetch,
			Tool:     f.ToolName,
			Duration: f.Took,
			Note:     f.Result.Summary,
		}
		if f.Err != nil {
			ev.Err = f.Err.Error()
			ev.Note = "failed"
		}
		trace.record(ev)
	}
	return out
}

// reasonOnce performs one LLM call. When strict is true, we prepend a
// reminder that the previous attempt cited a non-existent source.
func (a *Agent) reasonOnce(
	ctx context.Context,
	query, promptCtx string,
	history []llm.Message,
	trace *Trace,
	strict bool,
) (reasonOutput, error) {
	sys := systemPrompt
	if strict {
		sys += "\n\nIMPORTANT: Your previous attempt cited sources that do not exist. " +
			"Only use citation indices from the SOURCES list. If you cannot answer, refuse."
	}

	// Compose the user message. Conversation history goes first (so the
	// LLM sees the prior Q&A), then a single synthesised turn that
	// bundles this question's context + query.
	msgs := append([]llm.Message{}, history...)
	msgs = append(msgs, llm.Message{
		Role:    llm.RoleUser,
		Content: promptCtx + "\n\nQUESTION: " + query,
	})

	start := a.Now()
	resp, err := a.LLM.Complete(ctx, llm.Request{
		System:      sys,
		Messages:    msgs,
		Temperature: 0,
		MaxTokens:   2048,
		JSONSchema:  reasonSchema,
		SchemaName:  "ReasonOutput",
	})
	took := a.Now().Sub(start)

	ev := Event{
		Phase:    PhaseReason,
		Duration: took,
		Tokens:   resp.Usage.TotalTokens,
		Note:     "llm call",
	}
	if strict {
		ev.Note = "llm call (retry, stricter prompt)"
	}
	if err != nil {
		ev.Err = err.Error()
		trace.record(ev)
		return reasonOutput{}, fmt.Errorf("reason: %w", err)
	}
	trace.record(ev)

	var out reasonOutput
	if derr := llm.DecodeJSON(resp, &out); derr != nil {
		return reasonOutput{}, fmt.Errorf("reason: decode: %w", derr)
	}
	return out, nil
}

// shape validates grounding and builds the final Answer. The second
// return value indicates whether the caller should retry the Reason
// step with a stricter prompt.
func (a *Agent) shape(out reasonOutput, orderedSources []tools.Source, trace *Trace) (Answer, bool) {
	shapeStart := a.Now()
	defer func() {
		trace.record(Event{
			Phase:    PhaseShape,
			Duration: a.Now().Sub(shapeStart),
		})
	}()

	// Refusal path: no citations needed.
	if out.Refused {
		txt := strings.TrimSpace(firstNonEmpty(out.VerdictText, out.Answer))
		if txt == "" {
			txt = "I don't have a source for that."
		}
		return Answer{
			Refused:       true,
			Text:          txt,
			VerdictText:   txt,
			VerdictTone:   "red",
			RefusalReason: out.RefusalReason,
			Confidence:    ConfidenceLow,
		}, false
	}

	// Combine every place the model is allowed to put citations
	// (verdict headline + why bullets + legacy answer field) into one
	// blob for grounding validation. We don't care WHICH field the
	// citation appears in; we only care that every [n] resolves to a
	// real source.
	cited := citeBlob(out)

	// Validate inline [n] markers against the available source list.
	invalid, hasAny := validateCitations(cited, len(orderedSources))
	if len(invalid) > 0 {
		// Hallucinated source index — caller should retry once.
		return Answer{}, true
	}
	if !hasAny && len(orderedSources) > 0 {
		// Non-empty answer with no inline [n] markers anywhere. Two
		// recovery paths before we resort to refusing:
		//
		// 1. The model populated the structured "citations" field with
		//    valid indices. It just forgot the inline brackets. Trust
		//    the structured field, append the markers ourselves to the
		//    verdict line, and surface those sources. The grounding
		//    contract holds: the model declared which sources its
		//    claims came from.
		//
		// 2. Otherwise — fall through to a refusal.
		validStructured := filterValid(out.Citations, len(orderedSources))
		if len(validStructured) > 0 {
			headline := strings.TrimSpace(firstNonEmpty(out.VerdictText, out.Answer))
			headline = appendInlineMarkers(headline, validStructured)
			seen := map[int]bool{}
			var outSources []tools.Source
			for _, n := range validStructured {
				if seen[n] {
					continue
				}
				seen[n] = true
				outSources = append(outSources, orderedSources[n-1])
			}
			return Answer{
				Text:        headline,
				VerdictText: headline,
				VerdictTone: normaliseTone(out.VerdictTone),
				Why:         trimWhy(out.Why),
				NextActions: trimWhy(out.NextActions),
				Sources:     outSources,
				Confidence:  parseConfidence(out.Confidence),
			}, false
		}
		return Answer{
			Refused:       true,
			Text:          "I don't have a source for that.",
			VerdictText:   "I don't have a source for that.",
			VerdictTone:   "red",
			RefusalReason: "answer contained no citations despite sources being available",
			Confidence:    ConfidenceLow,
		}, false
	}

	// Build ordered, deduplicated source list for the final Answer.
	used := extractCitations(cited)
	seen := map[int]bool{}
	var outSources []tools.Source
	for _, n := range used {
		if seen[n] {
			continue
		}
		seen[n] = true
		outSources = append(outSources, orderedSources[n-1])
	}

	conf := parseConfidence(out.Confidence)
	headline := strings.TrimSpace(firstNonEmpty(out.VerdictText, out.Answer))
	return Answer{
		Text:        headline,
		VerdictText: headline,
		VerdictTone: normaliseTone(out.VerdictTone),
		Why:         trimWhy(out.Why),
		NextActions: trimWhy(out.NextActions),
		Sources:     outSources,
		Confidence:  conf,
	}, false
}

// citeBlob concatenates every field where the model is allowed to put
// inline [n] citations. Newline-joined so a citation at the end of one
// bullet doesn't bleed into the next during regex extraction.
func citeBlob(out reasonOutput) string {
	parts := []string{out.VerdictText, out.Answer}
	parts = append(parts, out.Why...)
	return strings.Join(parts, "\n")
}

// firstNonEmpty returns the first argument with non-whitespace content.
func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// trimWhy drops empty / whitespace-only entries so the renderer doesn't
// emit blank bullets when the model returns ["foo", ""].
func trimWhy(ss []string) []string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// normaliseTone collapses unexpected enum values to "info" so the
// shaper always gets a known tone. Azure's strict mode should already
// reject other values, but defence in depth doesn't hurt.
func normaliseTone(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "green", "yellow", "red", "info":
		return strings.ToLower(strings.TrimSpace(s))
	default:
		return "info"
	}
}

func parseConfidence(s string) Confidence {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "high":
		return ConfidenceHigh
	case "low":
		return ConfidenceLow
	default:
		return ConfidenceMedium
	}
}

// ErrNotConfigured is returned by callers when no Azure Foundry config
// is available and the REPL would otherwise route free-form text to a
// nil LLM. Kept here so REPL wiring can depend only on the agent pkg.
var ErrNotConfigured = errors.New("LLM not configured: set AZURE_FOUNDRY_ENDPOINT, AZURE_FOUNDRY_DEPLOYMENT, AZURE_FOUNDRY_API_KEY")
