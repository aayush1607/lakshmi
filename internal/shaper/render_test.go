package shaper

import (
	"strings"
	"testing"
	"time"

	
)

// fixedTime is the wall-clock used by every test that needs one. Picked
// to be far in the past so we can construct "FetchedAt within 24h"
// scenarios trivially.
var fixedTime = time.Date(2024, 4, 5, 10, 0, 0, 0, time.UTC)

// TestRenderInformationalLayout asserts the order and presence of every
// section for the most common answer kind: a sourced Q&A response.
func TestRenderInformationalLayout(t *testing.T) {
	a := Answer{
		Kind:        KindInformational,
		Verdict:     VerdictNone,
		VerdictText: "Your IT exposure is 100% of your portfolio [1].",
		Why: []string{
			"TCS and INFY together total ₹2,50,000 [1].",
			"Both map to the IT Services sector [2].",
		},
		Sources: []Source{
			{Name: "Zerodha Kite", URL: "kite.zerodha.com", Tier: 2, FetchedAt: fixedTime},
			{Name: "Lakshmi static sector map", Tier: 3, FetchedAt: fixedTime},
		},
		Confidence:  ComputeConfidence([]Source{{Tier: 2, FetchedAt: fixedTime}, {Tier: 3, FetchedAt: fixedTime}}, fixedTime),
		NextActions: []string{"/why", "/p --by pnl"},
		AsOf:        fixedTime,
	}
	out := Render(a, PlainTheme(), false)

	wantOrder := []string{
		"Your IT exposure",
		"--- WHY",
		"TCS and INFY",
		"sector",
		"--- SOURCES",
		"Zerodha Kite",
		"--- CONFIDENCE:",
		"based on 2 sources",
		"📎 next:",
		"/why",
		"/p --by pnl",
	}
	assertOrdered(t, out, wantOrder)

	if !strings.Contains(out, "[1] Zerodha Kite") || !strings.Contains(out, "[2] Lakshmi static sector map") {
		t.Errorf("sources missing or out of order: %q", out)
	}
}

// TestRenderRefusalLayout: refusals carry a red verdict, no sources,
// no confidence section, and an actionable hint.
func TestRenderRefusalLayout(t *testing.T) {
	a := Answer{
		Kind:        KindRefusal,
		VerdictText: "I don't have a source for that.",
		NextActions: []string{"/help"},
	}
	out := Render(a, PlainTheme(), false)

	if !strings.Contains(out, "[NO]") {
		t.Errorf("refusal missing red glyph: %q", out)
	}
	if strings.Contains(out, "CONFIDENCE") {
		t.Errorf("refusal should not show confidence: %q", out)
	}
	if strings.Contains(out, "SOURCES") {
		t.Errorf("refusal should not show sources block: %q", out)
	}
	if !strings.Contains(out, "/help") {
		t.Errorf("refusal missing next action: %q", out)
	}
}

// TestRenderUnknownCommand: friendly verdict + suggestion only.
func TestRenderUnknownCommand(t *testing.T) {
	a := Answer{
		Kind:        KindUnknown,
		VerdictText: "Unknown command: /foo",
		NextActions: []string{"/help"},
	}
	out := Render(a, PlainTheme(), false)

	if !strings.Contains(out, "[!!]") {
		t.Errorf("unknown should use yellow glyph: %q", out)
	}
	if strings.Contains(out, "WHY") || strings.Contains(out, "SOURCES") || strings.Contains(out, "CONFIDENCE") {
		t.Errorf("unknown command should only have verdict + next: %q", out)
	}
}

// TestRenderDetailCollapses: a detail block longer than the threshold
// should show the preview + a "type more" hint.
func TestRenderDetailCollapses(t *testing.T) {
	long := strings.Repeat("line\n", detailCollapseLines+5)
	a := Answer{
		Kind:        KindInformational,
		VerdictText: "Here are your holdings.",
		Detail:      long,
		Sources:     []Source{{Name: "Zerodha Kite", Tier: 2, FetchedAt: fixedTime}},
	}
	collapsed := Render(a, PlainTheme(), false)
	expanded := Render(a, PlainTheme(), true)

	if !strings.Contains(collapsed, "more lines") {
		t.Errorf("collapsed render should show 'more lines' hint: %q", collapsed)
	}
	if strings.Contains(expanded, "more lines") {
		t.Errorf("expanded render should NOT show hint: %q", expanded)
	}
	// Expanded output must contain every line.
	if strings.Count(expanded, "line\n") < detailCollapseLines+5 {
		t.Errorf("expanded render missing lines")
	}
}

// TestRenderShortDetailNotCollapsed: small detail is shown in full
// even when expand=false.
func TestRenderShortDetailNotCollapsed(t *testing.T) {
	short := strings.Repeat("row\n", 4)
	a := Answer{
		Kind:        KindInformational,
		VerdictText: "Two rows.",
		Detail:      short,
	}
	out := Render(a, PlainTheme(), false)
	if strings.Contains(out, "more lines") {
		t.Errorf("short detail must not collapse: %q", out)
	}
}

// TestRenderCitationMarkersTinted: every [n] in verdict and Why goes
// through the cite style. Plain theme renders identically; default
// theme wraps with ANSI (we only assert presence here, not bytes).
func TestRenderCitationMarkersTinted(t *testing.T) {
	a := Answer{
		Kind:        KindInformational,
		VerdictText: "X is Y [1].",
		Why:         []string{"reason A [1]"},
		Sources:     []Source{{Name: "Zerodha Kite", Tier: 2, FetchedAt: fixedTime}},
	}
	out := Render(a, PlainTheme(), false)
	if !strings.Contains(out, "[1]") {
		t.Errorf("citation marker missing: %q", out)
	}
}

// TestRenderDeterministic: same input → same output, byte for byte.
// This is the property that lets us golden-test confidently.
func TestRenderDeterministic(t *testing.T) {
	a := Answer{
		Kind:        KindInformational,
		VerdictText: "Your total invested is ₹2,50,000 [1].",
		Sources:     []Source{{Name: "Zerodha Kite", Tier: 2, FetchedAt: fixedTime}},
		Confidence:  85,
		AsOf:        fixedTime,
	}
	a1 := Render(a, PlainTheme(), false)
	a2 := Render(a, PlainTheme(), false)
	if a1 != a2 {
		t.Errorf("render not deterministic:\n--first--\n%s\n--second--\n%s", a1, a2)
	}
}

// TestComputeConfidence covers the scoring rules end-to-end.
func TestComputeConfidence(t *testing.T) {
	cases := []struct {
		name    string
		sources []Source
		now     time.Time
		want    int
	}{
		{"no sources", nil, fixedTime, 0},
		{"single tier 1", []Source{{Tier: 1, FetchedAt: fixedTime}}, fixedTime, 99},
		{"single tier 2", []Source{{Tier: 2, FetchedAt: fixedTime}}, fixedTime, 80},
		{"single tier 3", []Source{{Tier: 3, FetchedAt: fixedTime}}, fixedTime, 50},
		{"two tier 2 (count bonus)", []Source{{Tier: 2, FetchedAt: fixedTime}, {Tier: 2, FetchedAt: fixedTime}}, fixedTime, 85},
		{"stale t1 docked", []Source{{Tier: 1, FetchedAt: fixedTime}}, fixedTime.Add(48 * time.Hour), 90},
		{"floor at 10", []Source{{Tier: 99, FetchedAt: fixedTime}}, fixedTime.Add(48 * time.Hour), 20},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ComputeConfidence(tc.sources, tc.now)
			if got != tc.want {
				t.Errorf("ComputeConfidence = %d, want %d", got, tc.want)
			}
		})
	}
}

// assertOrdered checks each substring appears in `out` in the given order.
func assertOrdered(t *testing.T, out string, wants []string) {
	t.Helper()
	pos := 0
	for _, w := range wants {
		i := strings.Index(out[pos:], w)
		if i < 0 {
			t.Errorf("missing %q after position %d in:\n%s", w, pos, out)
			return
		}
		pos += i + len(w)
	}
}
