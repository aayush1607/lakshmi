// Package repl — UI styles. Centralised so the colour palette is
// consistent across banner, prompt, agent answers, and footer.
//
// Palette is on-brand "Indian markets":
//   - saffron/gold for the brand glyph and primary highlights,
//   - emerald for green/up/positive signals,
//   - rose for red/down/negative signals,
//   - cool grey for chrome and faint metadata.
//
// All colours use 256-colour codes so they degrade gracefully on
// terminals without truecolor.
package repl

import "github.com/charmbracelet/lipgloss"

// Brand colours.
var (
	colSaffron = lipgloss.Color("214") // saffron / marigold
	colGold    = lipgloss.Color("220") // bright gold
	colRose    = lipgloss.Color("203") // soft red
	colEmerald = lipgloss.Color("78")  // green
	colTeal    = lipgloss.Color("87")  // cyan accent
	colMauve   = lipgloss.Color("177") // accent for spinner
	colMuted   = lipgloss.Color("245") // mid grey
	colDim     = lipgloss.Color("240") // faint grey
)

// Reusable styles.
var (
	StyleBrand   = lipgloss.NewStyle().Foreground(colSaffron).Bold(true)
	StyleAccent  = lipgloss.NewStyle().Foreground(colGold).Bold(true)
	StyleSuccess = lipgloss.NewStyle().Foreground(colEmerald).Bold(true)
	StyleError   = lipgloss.NewStyle().Foreground(colRose).Bold(true)
	StyleHint    = lipgloss.NewStyle().Foreground(colMuted).Italic(true)
	StyleFaint   = lipgloss.NewStyle().Foreground(colDim)
	StylePrompt  = lipgloss.NewStyle().Foreground(colSaffron).Bold(true)
	StyleEcho    = lipgloss.NewStyle().Foreground(colMuted)
	StyleSpinner = lipgloss.NewStyle().Foreground(colMauve).Bold(true)
	StyleThink   = lipgloss.NewStyle().Foreground(colMauve).Italic(true)
	StyleTimer   = lipgloss.NewStyle().Foreground(colDim)
)
