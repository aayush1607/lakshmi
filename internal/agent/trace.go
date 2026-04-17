package agent

import (
	"fmt"
	"strings"
	"time"
)

// Phase names are stable strings used by Trace events and /why rendering.
const (
	PhasePlan   = "plan"
	PhaseFetch  = "fetch"
	PhaseReason = "reason"
	PhaseShape  = "shape"
)

// Event is a single line in the reasoning trace.
type Event struct {
	Phase    string        // one of the Phase* constants
	Tool     string        // empty unless Phase == PhaseFetch
	Note     string        // short human-readable context
	Duration time.Duration // wall-clock for this step
	Tokens   int           // tokens used (Reason phase only)
	Err      string        // populated when the step failed
}

// Trace is the append-only log of agent activity for a single Run.
// Rendering is the /why handler's job — the agent just collects events.
type Trace struct {
	Events []Event
	Query  string
	Total  time.Duration
}

// record appends an event. Not concurrency-safe; the agent runs Fetch
// in parallel via an errgroup but funnels results through a single
// channel that the main goroutine reads, so all Trace writes happen
// on the Run goroutine.
func (t *Trace) record(e Event) { t.Events = append(t.Events, e) }

// String renders the trace for /why. Format is deliberately compact:
// one line per event, aligned columns, no colour (the REPL layer can
// add styling if it wants).
func (t Trace) String() string {
	if len(t.Events) == 0 {
		return "(no trace)\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "query: %q\n", t.Query)
	fmt.Fprintf(&b, "total: %s\n", t.Total.Round(time.Millisecond))
	b.WriteString("events:\n")
	for i, e := range t.Events {
		label := e.Phase
		if e.Tool != "" {
			label = e.Phase + ":" + e.Tool
		}
		fmt.Fprintf(&b, "  %2d. %-22s %8s  %s",
			i+1,
			label,
			e.Duration.Round(time.Millisecond),
			e.Note,
		)
		if e.Tokens > 0 {
			fmt.Fprintf(&b, "  (%d tokens)", e.Tokens)
		}
		if e.Err != "" {
			fmt.Fprintf(&b, "  ERR: %s", e.Err)
		}
		b.WriteString("\n")
	}
	return b.String()
}
