// Package cache is Lakshmi's local data cache (F1.4).
//
// Design choices:
//
//   - File-per-entry layout at $LAKSHMI_HOME/data/<namespace>/<key>.json.
//     JSON is readable by humans and by `jq`, which matters for an
//     "auditable" local cache. No bespoke binary format, no embedded KV
//     server, no lock conflicts with other `lakshmi` processes.
//   - Writes are atomic via temp-file + rename, so a crashed process
//     never leaves a half-written entry.
//   - Freshness policy is a per-namespace function, not a single TTL.
//     Indian markets have hard session boundaries (09:15–15:30 IST),
//     and the spec calls for different behaviour inside vs outside the
//     session — a function captures that cleanly.
//   - Cache-free mode flips a single boolean on the Store; reads return
//     miss, writes are dropped. This means no code path needs to branch
//     on "is the cache on?" — the Store itself enforces it.
package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Namespaces used by Sprint 1 and planned for Sprint 2. Callers should
// prefer these constants over string literals so the Stats view stays
// in sync with actual writes.
const (
	NSHoldings     = "holdings"
	NSQuotes       = "quotes"
	NSFundamentals = "fundamentals"
	NSNews         = "news"
)

// Envelope is the on-disk shape of a cached entry.
type Envelope struct {
	Version   int             `json:"v"`
	Namespace string          `json:"ns"`
	Key       string          `json:"key"`
	FetchedAt time.Time       `json:"fetched_at"`
	Payload   json.RawMessage `json:"payload"`
}

// Entry is what Get returns: the decoded envelope plus a convenience Age.
type Entry struct {
	Envelope
	Age time.Duration
}

// FreshnessFn decides whether a cached entry is still fresh.
// `age` is the time since the entry was fetched; `marketOpen` is true
// during the 09:15–15:30 IST equity session (Mon–Fri).
type FreshnessFn func(age time.Duration, marketOpen bool) bool

// Rules holds the Sprint 1 freshness policies. These are the single
// source of truth — handlers, tests, and /cache status all consult this
// map so behaviour can never drift between layers.
var Rules = map[string]FreshnessFn{
	// Holdings: always refetch during a live session; off-hours cache
	// is good for one calendar day (the broker rewrites EOD positions).
	NSHoldings: func(age time.Duration, marketOpen bool) bool {
		if marketOpen {
			return false
		}
		return age < 24*time.Hour
	},
	// Quotes: always refetch live; off-hours the cached value IS the
	// last close, so any cached entry is acceptable regardless of age.
	NSQuotes: func(age time.Duration, marketOpen bool) bool {
		return !marketOpen
	},
	// Fundamentals: insensitive to market hours; refreshed hourly.
	NSFundamentals: func(age time.Duration, _ bool) bool {
		return age < time.Hour
	},
}

// Store is a file-backed cache rooted at a single directory.
//
// The zero value is NOT usable — call Open().
type Store struct {
	dir string

	mu      sync.RWMutex
	enabled bool
}

// Open creates (if needed) the cache directory and returns a Store.
// `enabled=false` puts the Store in no-cache mode: all Gets miss and
// all Puts are dropped without touching disk.
func Open(dir string, enabled bool) (*Store, error) {
	if enabled {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("cache mkdir: %w", err)
		}
	}
	return &Store{dir: dir, enabled: enabled}, nil
}

// Dir returns the root directory the store writes to. Useful for
// `/cache status` and for filesystem-snapshot tests.
func (s *Store) Dir() string { return s.dir }

// Enabled reports whether reads/writes hit disk.
func (s *Store) Enabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enabled
}

// SetEnabled toggles no-cache mode at runtime. When switching from
// enabled→disabled we do NOT remove existing data — use Clear() for
// that. Switching back re-enables reads and writes against whatever is
// on disk.
func (s *Store) SetEnabled(v bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v && s.dir != "" {
		if err := os.MkdirAll(s.dir, 0o700); err != nil {
			return fmt.Errorf("cache mkdir: %w", err)
		}
	}
	s.enabled = v
	return nil
}

// Get looks up an entry. Returns (entry, true, nil) on hit; returns
// (_, false, nil) on miss (including when the cache is disabled).
// Errors indicate a corrupted envelope or an I/O failure — callers
// should treat those as misses after logging.
func (s *Store) Get(namespace, key string) (Entry, bool, error) {
	if !s.Enabled() {
		return Entry{}, false, nil
	}
	path, err := s.path(namespace, key)
	if err != nil {
		return Entry{}, false, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Entry{}, false, nil
		}
		return Entry{}, false, err
	}
	var env Envelope
	if err := json.Unmarshal(b, &env); err != nil {
		return Entry{}, false, fmt.Errorf("decode cache envelope %s/%s: %w", namespace, key, err)
	}
	return Entry{Envelope: env, Age: time.Since(env.FetchedAt)}, true, nil
}

// Put serialises `v` into an envelope and atomically writes it.
// No-ops when the cache is disabled.
func (s *Store) Put(namespace, key string, v any) error {
	if !s.Enabled() {
		return nil
	}
	payload, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encode cache payload: %w", err)
	}
	env := Envelope{
		Version:   1,
		Namespace: namespace,
		Key:       key,
		FetchedAt: time.Now().UTC(),
		Payload:   payload,
	}
	body, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return err
	}
	path, err := s.path(namespace, key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Bust removes a single entry. No-op when absent or disabled.
func (s *Store) Bust(namespace, key string) error {
	if !s.Enabled() {
		return nil
	}
	path, err := s.path(namespace, key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// Clear wipes the entire cache directory. Safe when disabled (we still
// scrub existing files so the user gets what they expect).
func (s *Store) Clear() error {
	if s.dir == "" {
		return nil
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(s.dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// NamespaceStats summarises a single namespace's on-disk footprint.
type NamespaceStats struct {
	Namespace   string
	Entries     int
	Bytes       int64
	LastRefresh time.Time // most recent FetchedAt across entries
}

// Stats is the aggregate view surfaced by /cache status.
type Stats struct {
	Enabled    bool
	Dir        string
	Namespaces []NamespaceStats
	TotalBytes int64
}

// Stats scans the cache directory. It tolerates foreign files (e.g. the
// user poking around with an editor) by skipping anything it cannot
// parse as an envelope. Returns a Stats with Enabled=false for no-cache
// mode so renderers can show the "disabled" line without branching.
func (s *Store) Stats() (Stats, error) {
	out := Stats{Enabled: s.Enabled(), Dir: s.dir}
	if s.dir == "" {
		return out, nil
	}
	nsDirs, err := os.ReadDir(s.dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return out, nil
		}
		return out, err
	}
	for _, nd := range nsDirs {
		if !nd.IsDir() {
			continue
		}
		ns := NamespaceStats{Namespace: nd.Name()}
		nsPath := filepath.Join(s.dir, nd.Name())
		files, err := os.ReadDir(nsPath)
		if err != nil {
			return out, err
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
				continue
			}
			info, err := f.Info()
			if err != nil {
				continue
			}
			ns.Entries++
			ns.Bytes += info.Size()
			if b, err := os.ReadFile(filepath.Join(nsPath, f.Name())); err == nil {
				var env Envelope
				if json.Unmarshal(b, &env) == nil && env.FetchedAt.After(ns.LastRefresh) {
					ns.LastRefresh = env.FetchedAt
				}
			}
		}
		if ns.Entries == 0 {
			continue
		}
		out.Namespaces = append(out.Namespaces, ns)
		out.TotalBytes += ns.Bytes
	}
	sort.Slice(out.Namespaces, func(i, j int) bool {
		return out.Namespaces[i].Namespace < out.Namespaces[j].Namespace
	})
	return out, nil
}

// path resolves the on-disk file for (namespace, key). Both components
// are sanitised so a malicious key like "../../etc/passwd" cannot
// escape the cache directory.
func (s *Store) path(namespace, key string) (string, error) {
	ns := safeSegment(namespace)
	k := safeSegment(key)
	if ns == "" || k == "" {
		return "", fmt.Errorf("cache: empty namespace or key")
	}
	return filepath.Join(s.dir, ns, k+".json"), nil
}

// safeSegment returns a filesystem-safe version of s: anything outside
// [A-Za-z0-9._-] becomes an underscore. Empty input -> empty output.
func safeSegment(s string) string {
	if s == "" {
		return ""
	}
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z',
			c >= 'a' && c <= 'z',
			c >= '0' && c <= '9',
			c == '_', c == '-', c == '.':
			b = append(b, c)
		default:
			b = append(b, '_')
		}
	}
	// Defensive: refuse names that resolve to parent traversal.
	clean := string(b)
	if clean == "." || clean == ".." {
		return "_" + clean
	}
	return clean
}
