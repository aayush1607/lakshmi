package repl

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Banner returns the welcome banner shown when the REPL launches.
//
// It is a pure function so it can be golden-tested and rendered either
// with or without colour. The version argument is stamped into the
// output; pass version.Version at the call site.
func Banner(version string) string {
	if version == "" {
		version = "dev"
	}
	// Hide the git "-dirty" marker from the welcome banner; it's still
	// available via `lakshmi version` for debugging.
	version = strings.TrimSuffix(version, "-dirty")

	tape := lipgloss.NewStyle().Foreground(colDim).Render("   ▁ ▂ ▃ ▅ ▆ █  ") +
		lipgloss.NewStyle().Foreground(colEmerald).Render("▲ NIFTY  ") +
		lipgloss.NewStyle().Foreground(colEmerald).Render("▲ SENSEX  ") +
		lipgloss.NewStyle().Foreground(colRose).Render("▼ USDINR  ") +
		lipgloss.NewStyle().Foreground(colDim).Render("█ ▆ ▅ ▃ ▂ ▁")

	border := lipgloss.NewStyle().Foreground(colSaffron)
	logo := lipgloss.NewStyle().Foreground(colGold).Bold(true)
	verStyle := lipgloss.NewStyle().Foreground(colMuted).Italic(true)

	var b strings.Builder
	b.WriteString(tape + "\n")
	b.WriteString(border.Render("  ╭──────────────────────────────────────────────────────────╮") + "\n")
	b.WriteString(border.Render("  │  ") + logo.Render("   _       _         _           _                  ") + border.Render("    │") + "\n")
	b.WriteString(border.Render("  │  ") + logo.Render("  | | __ _| | _____| |__  _ __ ___ (_)              ") + border.Render("    │") + "\n")
	b.WriteString(border.Render("  │  ") + logo.Render("  | |/ _` | |/ / __| '_ \\| '_ ` _ \\| |              ") + border.Render("    │") + "\n")
	b.WriteString(border.Render("  │  ") + logo.Render("  | | (_| |   <\\__ \\ | | | | | | | | |              ") + border.Render("    │") + "\n")
	b.WriteString(border.Render("  │  ") + logo.Render("  |_|\\__,_|_|\\_\\___/_| |_|_| |_| |_|_|  ") + verStyle.Render(pad(version, 12)) + border.Render("    │") + "\n")
	b.WriteString(border.Render("  ╰──────────────────────────────────────────────────────────╯") + "\n")
	b.WriteString(StyleAccent.Render("     Stock market at your terminal.") + "\n")
	b.WriteString("\n")
	b.WriteString(StyleHint.Render("     Type ") + StyleAccent.Render("/help") +
		StyleHint.Render(" to list commands  ·  ") + StyleAccent.Render("/exit") +
		StyleHint.Render(" to quit.") + "\n")
	_ = fmt.Sprintf // keep fmt import if needed
	return b.String()
}

// pad right-pads s with spaces to the given width. If s is longer, it is
// returned as-is. Used to keep the banner's right border aligned regardless
// of version string length.
func pad(s string, width int) string {
	if len(s) >= width {
		return s + " "
	}
	return s + strings.Repeat(" ", width-len(s))
}

