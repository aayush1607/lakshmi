// Package main — process-wide wiring shared by the CLI subcommands and
// the REPL handlers. Keeping a single App instance means `/cache off`
// inside the REPL instantly takes effect for the next `/portfolio` —
// the Store pointer is the same one both look at.
package main

import (
	"os"
	"strings"
	"sync"

	"github.com/aayush1607/lakshmi/internal/cache"
	"github.com/aayush1607/lakshmi/internal/paths"
	"github.com/aayush1607/lakshmi/internal/shaper"
)

// App bundles the singletons every subcommand and REPL handler needs.
// It is built once per process (see buildApp) and then either used
// directly by a short-lived cobra command or captured by the REPL
// handler closures.
type App struct {
	Cache *cache.Store

	// lastMu guards lastAnswer. Handlers from different goroutines
	// (tea.Cmd callbacks) may write here concurrently with the /more
	// reader, so the lock is mandatory even for "trivial" pointer ops.
	lastMu     sync.Mutex
	lastAnswer *shaper.Answer
}

// SetLastAnswer stashes the most recent shaper.Answer so /more can
// re-render it with Detail expanded. Pass nil to clear (e.g. on a
// session reset).
func (a *App) SetLastAnswer(ans *shaper.Answer) {
	a.lastMu.Lock()
	a.lastAnswer = ans
	a.lastMu.Unlock()
}

// LastAnswer returns the most recently stashed Answer, or nil if none.
// Returns a copy of the pointer so the caller can render without
// holding the lock.
func (a *App) LastAnswer() *shaper.Answer {
	a.lastMu.Lock()
	defer a.lastMu.Unlock()
	return a.lastAnswer
}

// buildApp resolves cache-enablement from flags and env, then opens the
// Store. cacheFlag comes from the cobra root flag (`--no-cache` sets
// it to false); LAKSHMI_NO_CACHE=1 overrides if set.
func buildApp(cacheFlag bool) (*App, error) {
	if err := paths.EnsureHome(); err != nil {
		return nil, err
	}
	enabled := cacheFlag
	if v := strings.TrimSpace(os.Getenv("LAKSHMI_NO_CACHE")); v != "" && v != "0" && !strings.EqualFold(v, "false") {
		enabled = false
	}
	c, err := cache.Open(paths.Data(), enabled)
	if err != nil {
		return nil, err
	}
	return &App{Cache: c}, nil
}
