// Package main — `/cache` REPL handlers and `lakshmi cache` subcommand.
package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/aayush1607/lakshmi/internal/cache"
	"github.com/aayush1607/lakshmi/internal/repl"
)

// newCacheCmd wires `lakshmi cache status`, `lakshmi cache clear`, and
// `lakshmi cache on|off`. The on/off variants rewrite the env-less
// default for THIS process only — they are really convenience aliases
// for testing; the persistent opt-out is `LAKSHMI_NO_CACHE=1` or
// `--no-cache`.
func newCacheCmd(appRef **App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cache",
		Short: "inspect or clear the local data cache",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "show what is cached and how big it is",
		RunE: func(c *cobra.Command, args []string) error {
			fmt.Println(renderStats((*appRef).Cache))
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "clear",
		Short: "delete everything under ~/.lakshmi/data/",
		RunE: func(c *cobra.Command, args []string) error {
			if err := (*appRef).Cache.Clear(); err != nil {
				return err
			}
			fmt.Println("  ✓ cache cleared.")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "on",
		Short: "enable cache reads/writes for this process",
		RunE: func(c *cobra.Command, args []string) error {
			return (*appRef).Cache.SetEnabled(true)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "off",
		Short: "disable cache reads/writes for this process",
		RunE: func(c *cobra.Command, args []string) error {
			return (*appRef).Cache.SetEnabled(false)
		},
	})
	return cmd
}

// registerCacheHandlers wires /cache status|clear|on|off into the REPL.
func registerCacheHandlers(disp *repl.Dispatcher, app *App) {
	disp.Register(repl.Command{
		Name:    "/cache",
		Summary: "status | clear | on | off",
	}, func(input string) repl.Response {
		parts := strings.Fields(input)
		sub := ""
		if len(parts) > 1 {
			sub = strings.ToLower(parts[1])
		}
		switch sub {
		case "", "status":
			return repl.Response{Output: renderStats(app.Cache) + "\n"}
		case "clear":
			if err := app.Cache.Clear(); err != nil {
				return repl.Response{Output: "  ✗ clear failed: " + err.Error() + "\n"}
			}
			return repl.Response{Output: "  ✓ cache cleared.\n"}
		case "on":
			if err := app.Cache.SetEnabled(true); err != nil {
				return repl.Response{Output: "  ✗ " + err.Error() + "\n"}
			}
			return repl.Response{Output: "  ✓ cache enabled.\n"}
		case "off":
			_ = app.Cache.SetEnabled(false)
			return repl.Response{Output: "  ✓ cache disabled (no-cache mode) — fetches will always hit the source.\n"}
		default:
			return repl.Response{Output: "  usage: /cache [status|clear|on|off]\n"}
		}
	})
}

// renderStats is the shared formatter used by both the CLI and REPL.
// It reads from disk, so it is intentionally invoked on demand rather
// than memoised — the numbers must reflect the current state.
func renderStats(s *cache.Store) string {
	st, err := s.Stats()
	if err != nil && !errors.Is(err, errNoCacheDir) {
		return "  ✗ cache status: " + err.Error()
	}
	var b strings.Builder
	b.WriteString("━━━ CACHE ━━━\n")
	if !st.Enabled {
		b.WriteString("  status: disabled (no-cache mode)\n")
		b.WriteString("  dir:    " + st.Dir + "\n")
		return b.String()
	}
	b.WriteString("  status: enabled\n")
	b.WriteString("  dir:    " + st.Dir + "\n")
	if len(st.Namespaces) == 0 {
		b.WriteString("  (empty)\n")
		return b.String()
	}
	// Table. Two columns: namespace (left) and a packed summary.
	const nameW = 14
	for _, ns := range st.Namespaces {
		line := fmt.Sprintf("  %-*s %s   last refresh %s (%d entries)\n",
			nameW,
			ns.Namespace+":",
			humanBytes(ns.Bytes),
			humanSince(ns.LastRefresh),
			ns.Entries,
		)
		b.WriteString(line)
	}
	b.WriteString(fmt.Sprintf("  %-*s %s\n", nameW, "Total:", humanBytes(st.TotalBytes)))
	return b.String()
}

// errNoCacheDir is a sentinel reserved for the case where Stats() can
// legitimately short-circuit (currently unused; kept so the rendering
// switch above is easy to extend).
var errNoCacheDir = errors.New("no cache dir")

func humanBytes(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	case n < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	default:
		return fmt.Sprintf("%.1f GB", float64(n)/(1024*1024*1024))
	}
}

func humanSince(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d s ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d h ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%d d ago", int(d.Hours()/24))
	}
}
