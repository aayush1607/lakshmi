package agent

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// citePattern matches `[1]`, `[12]`, etc. We deliberately only accept
// a single integer per bracket; `[1, 2]` must be written as `[1][2]`.
var citePattern = regexp.MustCompile(`\[(\d+)\]`)

// extractCitations returns every [n] index that appears in text, in the
// order of first appearance, de-duplicated.
func extractCitations(text string) []int {
	matches := citePattern.FindAllStringSubmatch(text, -1)
	seen := map[int]bool{}
	out := make([]int, 0, len(matches))
	for _, m := range matches {
		n, err := strconv.Atoi(m[1])
		if err != nil || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

// validateCitations checks that every [n] in the answer text resolves
// to a source in the provided list. It returns the set of invalid
// indices (empty when the answer is fully grounded).
//
// We also require at least one citation when the answer is non-empty
// and not a refusal — an "ungrounded statement" is just as unsafe as
// a "badly-grounded statement".
func validateCitations(text string, numSources int) (invalid []int, hasAny bool) {
	used := extractCitations(text)
	for _, n := range used {
		if n < 1 || n > numSources {
			invalid = append(invalid, n)
		}
	}
	return invalid, len(used) > 0
}

// filterValid keeps only the indices that resolve to a real source.
func filterValid(idxs []int, numSources int) []int {
	out := make([]int, 0, len(idxs))
	seen := map[int]bool{}
	for _, n := range idxs {
		if n < 1 || n > numSources || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

// appendInlineMarkers tacks "[1][2]…" onto the end of text (before any
// trailing punctuation/newlines) so the rendered answer carries visible
// citations even when the model forgot to inline them. Used as a
// fallback when the structured citations field is populated but the
// answer text has no [n] markers.
func appendInlineMarkers(text string, idxs []int) string {
	if len(idxs) == 0 {
		return text
	}
	var marks string
	for _, n := range idxs {
		marks += fmt.Sprintf("[%d]", n)
	}
	t := strings.TrimRight(text, " \t\n")
	// Insert markers before a trailing period if the answer ends with one.
	if strings.HasSuffix(t, ".") {
		return t[:len(t)-1] + " " + marks + "."
	}
	return t + " " + marks
}
