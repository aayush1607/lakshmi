package shaper

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// citePattern matches inline citation markers like "[1]" or "[12]".
// Used to tint the brackets so the eye can pair claims with sources.
var citePattern = regexp.MustCompile(`\[\d+\]`)

// Render turns an Answer into a printable block in the universal
// Verdict-Why-Detail-Sources-Confidence-Next layout.
//
// The function is pure: same Answer + Theme always produces the same
// string. No I/O, no time.Now, no globals. This is what makes the
// golden tests possible and what lets the REPL re-render the same
// answer (e.g. when the user types `more`) without re-running tools.
//
// `expand` controls Detail collapsing: false (default) collapses long
// Detail to a preview + hint; true shows the whole thing. The REPL's
// /more handler flips this flag and re-renders the cached Answer.
func Render(a Answer, t Theme, expand bool) string {
	var b strings.Builder

	// 1. VERDICT — always present, always first.
	b.WriteString(renderVerdict(a, t))

	// 2. WHY — bullets backing the verdict.
	if len(a.Why) > 0 {
		b.WriteString("\n")
		b.WriteString(renderHeading("WHY", t))
		for _, w := range a.Why {
			line := tintCitations(strings.TrimSpace(w), t)
			b.WriteString("  " + t.StyleBullet.Render("•") + " " + line + "\n")
		}
	}

	// 3. DETAIL — long-form payload, collapsed past the threshold.
	if detail := strings.TrimRight(a.Detail, "\n"); detail != "" {
		b.WriteString("\n")
		b.WriteString(renderHeading("DETAIL", t))
		b.WriteString(renderDetail(detail, t, expand))
	}

	// 4. SOURCES — citeable evidence, in [n] order.
	if len(a.Sources) > 0 && a.Kind != KindUnknown {
		b.WriteString("\n")
		b.WriteString(renderHeading("SOURCES", t))
		for i, s := range a.Sources {
			b.WriteString(renderSource(i+1, s, t))
		}
	}

	// 5. CONFIDENCE — only when we actually have a number.
	if a.Confidence > 0 && a.Kind != KindUnknown && a.Kind != KindRefusal {
		b.WriteString("\n")
		b.WriteString(renderConfidence(a, t))
	}

	// 6. NEXT — runnable follow-ups.
	if len(a.NextActions) > 0 {
		b.WriteString("\n")
		b.WriteString(renderNext(a.NextActions, t))
	}

	return b.String()
}

// renderVerdict prints the top section: glyph + one-line text. For
// Refusals and Errors the verdict is mandatory and red; for Unknown
// commands it's yellow; for Informational answers it's the answer
// line itself with no glyph colour.
func renderVerdict(a Answer, t Theme) string {
	verdict := a.Verdict
	text := strings.TrimSpace(a.VerdictText)

	// Sensible defaults so callers can't accidentally print a blank
	// verdict block.
	switch a.Kind {
	case KindRefusal:
		if verdict == VerdictNone {
			verdict = VerdictRed
		}
		if text == "" {
			text = "I don't have a source for that."
		}
	case KindUnknown:
		if verdict == VerdictNone {
			verdict = VerdictYellow
		}
		if text == "" {
			text = "Unknown command."
		}
	case KindError:
		if verdict == VerdictNone {
			verdict = VerdictRed
		}
		if text == "" {
			text = "Something went wrong."
		}
	}

	glyph := t.glyph(verdict)
	style := t.verdictStyle(verdict)
	tinted := tintCitations(text, t)
	return style.Render(glyph) + "  " + style.Render(tinted) + "\n"
}

// renderHeading renders a section header like "─── WHY ───".
func renderHeading(label string, t Theme) string {
	return t.StyleHeading.Render(t.SectionRule+label) + "\n"
}

// renderDetail handles the collapse logic. Detail blocks already
// contain their own formatting (tables, prose) — the renderer only
// indents two spaces and optionally truncates.
func renderDetail(detail string, t Theme, expand bool) string {
	lines := strings.Split(detail, "\n")
	var b strings.Builder
	if !expand && len(lines) > detailCollapseLines {
		for _, ln := range lines[:detailPreviewLines] {
			b.WriteString("  " + ln + "\n")
		}
		hidden := len(lines) - detailPreviewLines
		hint := fmt.Sprintf("  … %d more lines — type %s to expand",
			hidden, t.StyleNext.Render("more"))
		b.WriteString(t.StyleDim.Render(hint) + "\n")
		return b.String()
	}
	for _, ln := range lines {
		b.WriteString("  " + ln + "\n")
	}
	return b.String()
}

// renderSource prints one source line with the trust-tier label.
func renderSource(idx int, s Source, t Theme) string {
	label := fmt.Sprintf("[%d]", idx)
	tier := tierLabel(s.Tier)
	desc := sourceExplanation(s)
	line := "  " + t.StyleCite.Render(label) + " " +
		t.StyleSource.Render(s.Name) + " " +
		t.StyleDim.Render("·") + " " +
		t.StyleBullet.Render(tier) + " " +
		t.StyleDim.Render("·") + " " +
		t.StyleBullet.Render(desc) + "\n"
	if s.URL != "" {
		line += "      " + t.StyleDim.Render(s.URL) + "\n"
	}
	return line
}

// renderConfidence renders the "─── CONFIDENCE: 87% ───" header plus a
// one-line "based on N sources · recency: 2h" sub-line.
func renderConfidence(a Answer, t Theme) string {
	style := confidenceStyle(a.Confidence, t)
	heading := t.StyleHeading.Render(t.SectionRule+"CONFIDENCE: ") +
		style.Render(fmt.Sprintf("%d%%", a.Confidence))
	parts := []string{fmt.Sprintf("based on %d source%s",
		len(a.Sources), pluralS(len(a.Sources)))}
	if !a.AsOf.IsZero() {
		parts = append(parts, "as of "+a.AsOf.Format("15:04 MST"))
	}
	sub := strings.Join(parts, " · ")
	return heading + "\n" + "  " + t.StyleDim.Render(sub) + "\n"
}

// renderNext lists suggested follow-up commands as a single line so
// the prompt is right under it. Each action is numbered ([1], [2], …)
// so the REPL can wire those numbers as one-keystroke shortcuts —
// users shouldn't have to retype "/p --by pnl" verbatim.
func renderNext(actions []string, t Theme) string {
	pretty := make([]string, len(actions))
	for i, a := range actions {
		num := t.StyleCite.Render(fmt.Sprintf("[%d]", i+1))
		pretty[i] = num + " " + t.StyleNext.Render(a)
	}
	hint := t.StyleDim.Render("  (type a number to run)")
	return t.StyleDim.Render("📎 next: ") +
		strings.Join(pretty, t.StyleDim.Render("   ")) +
		hint + "\n"
}

// tintCitations colours every [n] marker in a string so the eye can
// pair claims with the SOURCES list below.
func tintCitations(s string, t Theme) string {
	return citePattern.ReplaceAllStringFunc(s, func(m string) string {
		return t.StyleCite.Render(m)
	})
}

// confidenceStyle picks a colour for the percent number.
func confidenceStyle(score int, t Theme) lipgloss.Style {
	switch {
	case score >= 75:
		return t.StyleConfHi
	case score >= 45:
		return t.StyleConfMd
	default:
		return t.StyleConfLo
	}
}

// tierLabel translates the numeric tier into the user-facing word.
// Internal tier numbers ("Tier 3") leak too much taxonomy; users care
// about *trustworthiness*, not our naming scheme.
func tierLabel(tier int) string {
	switch tier {
	case 1:
		return "official"
	case 2:
		return "broker"
	case 3:
		return "derived"
	default:
		return fmt.Sprintf("tier %d", tier)
	}
}

// sourceExplanation returns a one-line, plain-English description of
// where the data came from. Hand-rolled cases for sources we know;
// generic tier descriptions otherwise.
func sourceExplanation(s Source) string {
	switch s.Name {
	case "Zerodha Kite":
		return "your live Zerodha account (same numbers as Kite)"
	case "Lakshmi static sector map", "Lakshmi sector map":
		return "built-in NSE symbol → sector lookup"
	}
	switch s.Tier {
	case 1:
		return "official primary source (exchange, regulator, or filing)"
	case 2:
		return "your broker or another regulated intermediary"
	case 3:
		return "derived or static reference table"
	default:
		return "unknown source"
	}
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// AgeOf is a small helper for callers that want to set Answer.AsOf
// without importing time directly. (Convenience; no behaviour.)
func AgeOf(t time.Time) time.Time { return t }
