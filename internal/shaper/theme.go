// Package shaper — theme: colours, glyphs, and layout knobs.
//
// The Theme is the only thing tests need to vary to assert layout
// (colour-stripped, narrower width, ASCII-only). Everything else in
// the renderer is driven by Answer.
package shaper

import "github.com/charmbracelet/lipgloss"

// detailCollapseLines — Detail blocks longer than this collapse to a
// preview + "type 'more' to expand" hint. Picked at 20 because it's
// roughly one terminal screen on a laptop and keeps the verdict +
// sources visible without scrolling.
const detailCollapseLines = 20

// detailPreviewLines — when collapsed, this many lines are shown
// before the "more" hint. Smaller than the collapse threshold so the
// preview always feels like a teaser, not a near-complete dump.
const detailPreviewLines = 6

// Theme bundles colour styles + glyph choices. Built once per render
// call by NewTheme and threaded through.
type Theme struct {
	// Verdict glyphs. Unicode by default; ASCII fallback toggled by
	// Theme.ASCII for non-UTF terminals.
	GlyphGreen  string
	GlyphYellow string
	GlyphRed    string
	GlyphInfo   string

	// Section heading prefix. Blank renders headings as plain caps;
	// the default uses a thin horizontal rule.
	SectionRule string

	// Lipgloss styles per verdict and chrome element.
	StyleGreen   lipgloss.Style
	StyleYellow  lipgloss.Style
	StyleRed     lipgloss.Style
	StyleInfo    lipgloss.Style
	StyleHeading lipgloss.Style
	StyleBullet  lipgloss.Style
	StyleCite    lipgloss.Style
	StyleSource  lipgloss.Style
	StyleDim     lipgloss.Style
	StyleNext    lipgloss.Style
	StyleConfHi  lipgloss.Style
	StyleConfMd  lipgloss.Style
	StyleConfLo  lipgloss.Style
}

// Brand palette — kept in sync with internal/repl/styles.go. Duplicated
// (rather than imported) so the shaper has no dependency on the REPL.
var (
	colSaffron = lipgloss.Color("214")
	colGold    = lipgloss.Color("220")
	colEmerald = lipgloss.Color("78")
	colRose    = lipgloss.Color("203")
	colAmber   = lipgloss.Color("214")
	colMuted   = lipgloss.Color("245")
	colDim     = lipgloss.Color("240")
)

// DefaultTheme returns the on-screen theme used by the REPL and the
// `lakshmi …` one-shot subcommands.
func DefaultTheme() Theme {
	return Theme{
		GlyphGreen:   "🟢",
		GlyphYellow:  "🟡",
		GlyphRed:     "🔴",
		GlyphInfo:    "•",
		SectionRule:  "─── ",
		StyleGreen:   lipgloss.NewStyle().Foreground(colEmerald).Bold(true),
		StyleYellow:  lipgloss.NewStyle().Foreground(colAmber).Bold(true),
		StyleRed:     lipgloss.NewStyle().Foreground(colRose).Bold(true),
		StyleInfo:    lipgloss.NewStyle().Foreground(colSaffron).Bold(true),
		StyleHeading: lipgloss.NewStyle().Foreground(colSaffron).Bold(true),
		StyleBullet:  lipgloss.NewStyle().Foreground(colMuted),
		StyleCite:    lipgloss.NewStyle().Foreground(colMuted),
		StyleSource:  lipgloss.NewStyle().Bold(true),
		StyleDim:     lipgloss.NewStyle().Foreground(colDim).Italic(true),
		StyleNext:    lipgloss.NewStyle().Foreground(colGold),
		StyleConfHi:  lipgloss.NewStyle().Foreground(colEmerald).Bold(true),
		StyleConfMd:  lipgloss.NewStyle().Foreground(colGold).Bold(true),
		StyleConfLo:  lipgloss.NewStyle().Foreground(colRose).Bold(true),
	}
}

// PlainTheme returns a colour-stripped, ASCII-only theme. Used by
// golden tests and by `lakshmi --no-color` (planned).
func PlainTheme() Theme {
	plain := lipgloss.NewStyle()
	return Theme{
		GlyphGreen:   "[OK]",
		GlyphYellow:  "[!!]",
		GlyphRed:     "[NO]",
		GlyphInfo:    "[i]",
		SectionRule:  "--- ",
		StyleGreen:   plain,
		StyleYellow:  plain,
		StyleRed:     plain,
		StyleInfo:    plain,
		StyleHeading: plain,
		StyleBullet:  plain,
		StyleCite:    plain,
		StyleSource:  plain,
		StyleDim:     plain,
		StyleNext:    plain,
		StyleConfHi:  plain,
		StyleConfMd:  plain,
		StyleConfLo:  plain,
	}
}

// glyph returns the lead glyph for a Verdict in this theme.
func (t Theme) glyph(v Verdict) string {
	switch v {
	case VerdictGreen:
		return t.GlyphGreen
	case VerdictYellow:
		return t.GlyphYellow
	case VerdictRed:
		return t.GlyphRed
	default:
		return t.GlyphInfo
	}
}

// verdictStyle returns the lipgloss style for the verdict line text.
func (t Theme) verdictStyle(v Verdict) lipgloss.Style {
	switch v {
	case VerdictGreen:
		return t.StyleGreen
	case VerdictYellow:
		return t.StyleYellow
	case VerdictRed:
		return t.StyleRed
	default:
		return t.StyleInfo
	}
}
