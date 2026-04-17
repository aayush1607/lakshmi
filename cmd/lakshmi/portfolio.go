// Package main — portfolio wiring for F1.3/F1.4. Exposes
// `lakshmi portfolio` and the REPL handlers `/portfolio`, `/p`,
// `/holdings` with cache-first semantics.
//
// Cache policy (F1.4 freshness rules):
//   - Market hours: always refetch.
//   - Off-hours:    serve cache if < 24h old; otherwise refetch.
//   - --fresh flag: always refetch, bypasses cache; still writes back.
//   - --no-cache:   no reads, no writes. Every call goes to the source.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/aayush1607/lakshmi/internal/broker"
	"github.com/aayush1607/lakshmi/internal/cache"
	"github.com/aayush1607/lakshmi/internal/portfolio"
	"github.com/aayush1607/lakshmi/internal/repl"
	"github.com/aayush1607/lakshmi/internal/shaper"
)

// holdingsTimeout bounds a single Kite holdings call. 10s is generous
// compared with Kite's typical ~200ms response; enough to absorb slow
// networks without hanging the REPL.
const holdingsTimeout = 10 * time.Second

// cacheKey is the on-disk key for holdings. One entry per user, so the
// same machine can log into multiple Zerodha accounts without cache
// collisions. "default" is the fallback when we don't have a session
// yet (which shouldn't happen — Holdings needs auth — but belt + braces).
func cacheKey(sess broker.Session) string {
	if sess.UserID != "" {
		return sess.UserID
	}
	return "default"
}

// newPortfolioCmd wires the cobra `lakshmi portfolio` subcommand.
func newPortfolioCmd(appRef **App) *cobra.Command {
	var sortFlag string
	var freshFlag bool
	cmd := &cobra.Command{
		Use:     "portfolio",
		Aliases: []string{"holdings", "p"},
		Short:   "show current Zerodha holdings",
		RunE: func(cmd *cobra.Command, args []string) error {
			by, err := portfolio.ParseSortBy(sortFlag)
			if err != nil {
				return err
			}
			app := *appRef
			out, err := loadAndRenderHoldings(cmd.Context(), app, by, freshFlag)
			if err != nil {
				return err
			}
			fmt.Println(out)
			return nil
		},
	}
	cmd.Flags().StringVar(&sortFlag, "by", "weight", "sort column: weight | pnl | symbol")
	cmd.Flags().BoolVar(&freshFlag, "fresh", false, "bypass cache and refetch from Zerodha")
	return cmd
}

// registerPortfolioHandlers adds /portfolio (and its aliases) to the
// dispatcher. The fetch runs in a tea.Cmd so the REPL stays responsive
// even when the cache is cold and we round-trip to Zerodha.
func registerPortfolioHandlers(disp *repl.Dispatcher, app *App) {
	h := func(input string) repl.Response {
		by, fresh, err := parsePortfolioArgs(input)
		if err != nil {
			return repl.Response{Output: "  ✗ " + err.Error() + "\n"}
		}

		// Fast path: cache hit means we can render synchronously without
		// spawning a tea.Cmd — meeting the <200ms acceptance criterion.
		if !fresh && app.Cache.Enabled() {
			if out, ok := tryRenderFromCache(app, by); ok {
				return repl.Response{Output: out + "\n"}
			}
		}

		msg := "  ⟳ Fetching holdings from Zerodha…\n"
		return repl.Response{
			Output: msg,
			Follow: holdingsCmd(app, by, fresh),
		}
	}
	disp.Register(repl.Command{
		Name:    "/portfolio",
		Summary: "show your holdings — flags: --by weight|pnl|symbol, --fresh",
	}, h)
	disp.RegisterAlias("/p", h)
	disp.RegisterAlias("/holdings", h)
}

// parsePortfolioArgs reads the inline `--by` and `--fresh` flags from
// the REPL input. Bare tokens are treated as sort values for muscle
// memory ("/p pnl").
func parsePortfolioArgs(input string) (portfolio.SortBy, bool, error) {
	parts := strings.Fields(input)
	by := portfolio.SortByWeight
	fresh := false
	for i := 1; i < len(parts); i++ {
		p := parts[i]
		switch {
		case p == "--fresh":
			fresh = true
		case p == "--by" && i+1 < len(parts):
			v, err := portfolio.ParseSortBy(parts[i+1])
			if err != nil {
				return by, fresh, err
			}
			by = v
			i++
		case strings.HasPrefix(p, "--by="):
			v, err := portfolio.ParseSortBy(strings.TrimPrefix(p, "--by="))
			if err != nil {
				return by, fresh, err
			}
			by = v
		case !strings.HasPrefix(p, "-"):
			v, err := portfolio.ParseSortBy(p)
			if err != nil {
				return by, fresh, err
			}
			by = v
		}
	}
	return by, fresh, nil
}

// loadAndRenderHoldings is the shared path used by the cobra subcommand.
// REPL handlers take a different path to keep the async behaviour clean.
func loadAndRenderHoldings(ctx context.Context, app *App, by portfolio.SortBy, fresh bool) (string, error) {
	// Try cache first (honours --fresh and no-cache).
	if !fresh && app.Cache.Enabled() {
		if out, ok := tryRenderFromCache(app, by); ok {
			return out, nil
		}
	}
	b, err := newBroker()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, holdingsTimeout)
	defer cancel()
	h, err := b.Holdings(ctx)
	if err != nil {
		return "", translateHoldingsErr(err)
	}
	sess, _ := b.Session()
	if err := app.Cache.Put(cache.NSHoldings, cacheKey(sess), h); err != nil {
		// Cache write failure is non-fatal — user still gets their data.
		// Surface as a subtle note so repeated failures are visible.
		fmt.Fprintln(os.Stderr, "  (cache write failed:", err, ")")
	}
	return renderHoldings(app, h, portfolio.Options{
		Sort:   by,
		AsOf:   time.Now(),
		Live:   portfolio.MarketOpen(time.Now()),
		Colour: true,
	}), nil
}

// renderHoldings runs holdings through the shaper and stashes the
// Answer for /more. Centralised so every code path (cobra, REPL cache
// hit, REPL fresh fetch) produces the same shape.
func renderHoldings(app *App, h []broker.Holding, opts portfolio.Options) string {
	ans := portfolio.Shape(h, opts)
	if app != nil {
		app.SetLastAnswer(&ans)
	}
	return shaper.Render(ans, shaper.DefaultTheme(), false)
}

// tryRenderFromCache attempts to serve holdings from disk without any
// network call. Returns (rendered, true) only when the freshness rule
// says the cached entry is still acceptable.
func tryRenderFromCache(app *App, by portfolio.SortBy) (string, bool) {
	// We don't know the user id without loading the session, but that's
	// a fast file read — no Kite call involved.
	b, err := newBroker()
	if err != nil {
		return "", false
	}
	sess, err := b.Session()
	if err != nil {
		return "", false
	}
	entry, hit, err := app.Cache.Get(cache.NSHoldings, cacheKey(sess))
	if err != nil || !hit {
		return "", false
	}
	marketOpen := portfolio.MarketOpen(time.Now())
	rule := cache.Rules[cache.NSHoldings]
	if !rule(entry.Age, marketOpen) {
		return "", false
	}
	var h []broker.Holding
	if err := json.Unmarshal(entry.Payload, &h); err != nil {
		return "", false
	}
	return renderHoldings(app, h, portfolio.Options{
		Sort:      by,
		AsOf:      entry.FetchedAt,
		Live:      marketOpen,
		Colour:    true,
		FromCache: true,
		CacheAge:  entry.Age,
	}), true
}

// holdingsCmd runs a fresh Kite fetch inside a tea.Cmd and produces a
// TranscriptMsg for the REPL.
func holdingsCmd(app *App, by portfolio.SortBy, fresh bool) tea.Cmd {
	_ = fresh // passed to loadAndRenderHoldings via bypass above
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), holdingsTimeout)
		defer cancel()
		b, err := newBroker()
		if err != nil {
			return repl.TranscriptMsg{Text: "  ✗ " + err.Error()}
		}
		h, err := b.Holdings(ctx)
		if err != nil {
			return repl.TranscriptMsg{Text: "  ✗ " + translateHoldingsErr(err).Error()}
		}
		if sess, serr := b.Session(); serr == nil {
			_ = app.Cache.Put(cache.NSHoldings, cacheKey(sess), h)
		}
		return repl.TranscriptMsg{Text: renderHoldings(app, h, portfolio.Options{
			Sort:   by,
			AsOf:   time.Now(),
			Live:   portfolio.MarketOpen(time.Now()),
			Colour: true,
		})}
	}
}

// translateHoldingsErr turns broker-level sentinels into user-friendly
// messages pointing at the next action.
func translateHoldingsErr(err error) error {
	switch {
	case errors.Is(err, broker.ErrNotLoggedIn):
		return errors.New("not logged in. Run `/login` or `lakshmi login`")
	case errors.Is(err, broker.ErrSessionExpired):
		return errors.New("session expired (Kite tokens reset at 06:00 IST). Run `/login` to refresh")
	default:
		return fmt.Errorf("holdings: %w", err)
	}
}
