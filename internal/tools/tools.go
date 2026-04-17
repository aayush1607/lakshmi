// Package tools defines the typed capabilities the agent can invoke.
//
// A Tool is the atomic unit of grounding: each invocation returns a
// structured Result *and* the Sources those facts came from. The agent
// serialises Results into the Reason prompt with numbered indices so
// the LLM can cite by index; the shaper then pulls the actual source
// metadata back out by those same indices. The LLM never invents
// sources — it only references what the tools actually fetched.
package tools

import (
	"context"
	"sort"
	"time"
)

// Source is the attribution for a single fact or fact cluster.
type Source struct {
	Name      string    `json:"name"`
	URL       string    `json:"url,omitempty"`
	Tier      int       `json:"tier"` // 1 = official/primary, 2 = broker/regulated, 3 = derived/static
	FetchedAt time.Time `json:"fetched_at"`
}

// Result is a tool's output.
type Result struct {
	// Data is the structured payload. It MUST be JSON-marshalable:
	// the agent serialises it verbatim into the Reason prompt, and it
	// must be cite-able by the LLM via [n] markers (where n = the
	// source index).
	Data any
	// Summary is a short human-readable note. Rendered into /why so
	// users see what each tool did without decoding JSON.
	Summary string
	// Sources lists every source the Data drew on. An empty slice is a
	// valid signal that this tool contributes no ground truth (e.g.
	// time_now is metadata, not a fact worth citing).
	Sources []Source
}

// Tool is something the agent can call.
type Tool interface {
	// Name is the stable identifier used in traces and (later) tool-calling.
	Name() string
	// Description is shown in /help and eventually in LLM tool-specs.
	Description() string
	// Call executes the tool. It should respect ctx.Done().
	Call(ctx context.Context, args map[string]any) (Result, error)
}

// Registry is a small map of tools keyed by name.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry builds an empty registry.
func NewRegistry() *Registry { return &Registry{tools: map[string]Tool{}} }

// Register adds a tool. Duplicate names replace the prior entry so
// tests can swap in fakes without ceremony.
func (r *Registry) Register(t Tool) { r.tools[t.Name()] = t }

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// Names returns tool names sorted alphabetically.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.tools))
	for n := range r.tools {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
