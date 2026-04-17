package repl

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// MaxHistory caps the number of entries stored on disk and in memory.
// Older entries are discarded when the cap is exceeded.
const MaxHistory = 1000

// History is an in-memory + file-backed list of prior REPL entries.
//
// The zero value is not usable; construct one with NewHistory. Concurrent
// use is safe (Bubbletea is single-threaded, but async commands may Append
// from a goroutine).
type History struct {
	path string

	mu      sync.Mutex
	entries []string
	cursor  int // index returned by Prev/Next; == len(entries) means "no selection"
}

// NewHistory opens (or lazily creates) a history file at path and loads
// its contents. Duplicate entries and empty lines are dropped on load.
//
// A missing file is not an error; an empty History is returned.
func NewHistory(path string) (*History, error) {
	h := &History{path: path}
	if err := h.load(); err != nil {
		return nil, err
	}
	h.cursor = len(h.entries)
	return h, nil
}

func (h *History) load() error {
	f, err := os.Open(h.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	defer f.Close()

	seen := make(map[string]struct{})
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for s.Scan() {
		line := s.Text()
		if line == "" {
			continue
		}
		if _, dup := seen[line]; dup {
			continue
		}
		seen[line] = struct{}{}
		h.entries = append(h.entries, line)
	}
	if err := s.Err(); err != nil {
		return err
	}
	if len(h.entries) > MaxHistory {
		h.entries = h.entries[len(h.entries)-MaxHistory:]
	}
	return nil
}

// Append adds an entry to history (in memory and on disk).
// Blank entries and exact duplicates of the most recent entry are ignored.
func (h *History) Append(entry string) error {
	if entry == "" {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	if n := len(h.entries); n > 0 && h.entries[n-1] == entry {
		h.cursor = len(h.entries)
		return nil
	}
	h.entries = append(h.entries, entry)
	if len(h.entries) > MaxHistory {
		h.entries = h.entries[len(h.entries)-MaxHistory:]
	}
	h.cursor = len(h.entries)
	return h.persist()
}

func (h *History) persist() error {
	if h.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(h.path), 0o700); err != nil {
		return err
	}
	tmp := h.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	for _, e := range h.entries {
		if _, err := w.WriteString(e + "\n"); err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
			return err
		}
	}
	if err := w.Flush(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, h.path)
}

// Entries returns a copy of all stored entries, oldest first.
func (h *History) Entries() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, len(h.entries))
	copy(out, h.entries)
	return out
}

// Prev moves the cursor one step back in history and returns the entry
// at the new position. It returns ("", false) when there is nothing
// earlier to show.
func (h *History) Prev() (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.entries) == 0 || h.cursor == 0 {
		return "", false
	}
	h.cursor--
	return h.entries[h.cursor], true
}

// Next moves the cursor one step forward. When it advances past the newest
// entry, it returns ("", true) to signal "clear the input".
func (h *History) Next() (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cursor >= len(h.entries) {
		return "", false
	}
	h.cursor++
	if h.cursor == len(h.entries) {
		return "", true
	}
	return h.entries[h.cursor], true
}

// ResetCursor places the cursor past the newest entry, so the next call
// to Prev returns the most recent entry.
func (h *History) ResetCursor() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cursor = len(h.entries)
}
