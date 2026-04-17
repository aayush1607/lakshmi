package repl

import (
	"strings"
	"unicode/utf8"
)

// wrapText soft-wraps s to width columns, breaking at spaces when
// possible and hard-breaking words longer than the width. Existing
// newlines in s are preserved. Tabs are treated as a single column —
// good enough for the REPL transcript; we don't render tab-aligned
// tables here.
//
// width <= 0 returns s unchanged. We use rune width (not byte) so the
// rupee glyph and other multi-byte characters count as one column.
func wrapText(s string, width int) string {
	if width <= 0 {
		return s
	}
	var out strings.Builder
	out.Grow(len(s))
	for i, line := range strings.Split(s, "\n") {
		if i > 0 {
			out.WriteByte('\n')
		}
		wrapLine(&out, line, width)
	}
	return out.String()
}

func wrapLine(out *strings.Builder, line string, width int) {
	if utf8.RuneCountInString(line) <= width {
		out.WriteString(line)
		return
	}
	// Preserve leading whitespace as the line's indent. This keeps
	// indented bullet lists ("  [1] …") looking right after wrap.
	indent := ""
	for _, r := range line {
		if r != ' ' && r != '\t' {
			break
		}
		indent += string(r)
	}
	rest := strings.TrimLeft(line, " \t")

	col := utf8.RuneCountInString(indent)
	out.WriteString(indent)
	first := true
	for _, word := range strings.Fields(rest) {
		w := utf8.RuneCountInString(word)
		// Word longer than width: hard-break it across lines.
		if w > width-len(indent) {
			if !first {
				out.WriteByte('\n')
				out.WriteString(indent)
				col = utf8.RuneCountInString(indent)
			}
			for _, chunk := range chunkRunes(word, width-utf8.RuneCountInString(indent)) {
				out.WriteString(chunk)
				out.WriteByte('\n')
				out.WriteString(indent)
				col = utf8.RuneCountInString(indent)
			}
			first = true
			continue
		}
		need := w
		if !first {
			need++ // for the separating space
		}
		if col+need > width {
			out.WriteByte('\n')
			out.WriteString(indent)
			col = utf8.RuneCountInString(indent)
			first = true
		}
		if !first {
			out.WriteByte(' ')
			col++
		}
		out.WriteString(word)
		col += w
		first = false
	}
}

func chunkRunes(s string, n int) []string {
	if n <= 0 {
		return []string{s}
	}
	var out []string
	var b strings.Builder
	count := 0
	for _, r := range s {
		b.WriteRune(r)
		count++
		if count >= n {
			out = append(out, b.String())
			b.Reset()
			count = 0
		}
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}
