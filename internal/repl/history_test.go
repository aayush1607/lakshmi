package repl

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestHistoryAppendDedupesConsecutive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history")
	h, err := NewHistory(path)
	if err != nil {
		t.Fatal(err)
	}

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(h.Append("/portfolio"))
	must(h.Append("/portfolio"))
	must(h.Append("/help"))

	want := []string{"/portfolio", "/help"}
	if got := h.Entries(); !reflect.DeepEqual(got, want) {
		t.Fatalf("entries = %v, want %v", got, want)
	}
}

func TestHistoryAppendIgnoresBlank(t *testing.T) {
	h, err := NewHistory(filepath.Join(t.TempDir(), "history"))
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Append(""); err != nil {
		t.Fatal(err)
	}
	if got := h.Entries(); len(got) != 0 {
		t.Fatalf("expected empty history, got %v", got)
	}
}

func TestHistoryPersistsAcrossReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history")

	h1, err := NewHistory(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range []string{"/help", "/portfolio", "ask anything"} {
		if err := h1.Append(e); err != nil {
			t.Fatal(err)
		}
	}

	h2, err := NewHistory(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/help", "/portfolio", "ask anything"}
	if got := h2.Entries(); !reflect.DeepEqual(got, want) {
		t.Fatalf("reloaded = %v, want %v", got, want)
	}
}

func TestHistoryDropsDuplicatesOnLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history")
	// Write a file with duplicates and blanks to ensure load dedupes.
	raw := "/help\n\n/help\n/portfolio\n/help\n"
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}

	h, err := NewHistory(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/help", "/portfolio"}
	if got := h.Entries(); !reflect.DeepEqual(got, want) {
		t.Fatalf("entries = %v, want %v", got, want)
	}
}

func TestHistoryPrevNextNavigation(t *testing.T) {
	h, err := NewHistory(filepath.Join(t.TempDir(), "history"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range []string{"one", "two", "three"} {
		_ = h.Append(e)
	}

	// Cursor starts past newest; Next must be a no-op at that boundary.
	if _, ok := h.Next(); ok {
		t.Fatal("Next at newest should return ok=false")
	}

	// Walk back.
	if got, _ := h.Prev(); got != "three" {
		t.Fatalf("Prev 1 = %q", got)
	}
	if got, _ := h.Prev(); got != "two" {
		t.Fatalf("Prev 2 = %q", got)
	}
	if got, _ := h.Prev(); got != "one" {
		t.Fatalf("Prev 3 = %q", got)
	}
	if _, ok := h.Prev(); ok {
		t.Fatal("Prev beyond oldest should return ok=false")
	}

	// Walk forward.
	if got, _ := h.Next(); got != "two" {
		t.Fatalf("forward 1 = %q", got)
	}
	if got, _ := h.Next(); got != "three" {
		t.Fatalf("forward 2 = %q", got)
	}
	// Moving past newest clears the prompt: value "", ok=true.
	got, ok := h.Next()
	if got != "" || !ok {
		t.Fatalf("past-newest = (%q,%v), want (\"\",true)", got, ok)
	}
}
