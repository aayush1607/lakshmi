package repl

import (
	"fmt"
	"strings"
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
	var b strings.Builder
	//  Small "Slant"-style logo + a market tape motif above it.
	b.WriteString("   ▁ ▂ ▃ ▅ ▆ █   ▲  NIFTY  ▲  SENSEX  ▼  USDINR   █ ▆ ▅ ▃ ▂ ▁\n")
	b.WriteString("  ╭──────────────────────────────────────────────────────────╮\n")
	b.WriteString("  │   _       _         _           _                        │\n")
	b.WriteString("  │  | | __ _| | _____| |__  _ __ ___ (_)                    │\n")
	b.WriteString("  │  | |/ _` | |/ / __| '_ \\| '_ ` _ \\| |                    │\n")
	b.WriteString("  │  | | (_| |   <\\__ \\ | | | | | | | | |                    │\n")
	b.WriteString("  │  |_|\\__,_|_|\\_\\___/_| |_|_| |_| |_|_|   " + pad(version, 15) + "│\n")
	b.WriteString("  ╰──────────────────────────────────────────────────────────╯\n")
	b.WriteString("     Stock market at your terminal.\n")
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("     Type %s to list commands  ·  %s to quit.\n", "/help", "/exit"))
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

