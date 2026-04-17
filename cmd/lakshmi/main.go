// Command lakshmi is the entry point for the Lakshmi CLI and interactive
// shell.
//
// Subcommands:
//
//	lakshmi                 launch the interactive REPL (default)
//	lakshmi repl            same as above, explicit
//	lakshmi version         print build version
//	lakshmi login/logout    Zerodha browser OAuth (F1.2)
//	lakshmi session         current login status
//	lakshmi portfolio       holdings table (F1.3)
//	lakshmi cache …         inspect/clear the local data cache (F1.4)
//
// Global flag `--no-cache` (or env `LAKSHMI_NO_CACHE=1`) disables the
// local cache for this process: no reads from disk, no writes to disk.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/aayush1607/lakshmi/internal/paths"
	"github.com/aayush1607/lakshmi/internal/repl"
	"github.com/aayush1607/lakshmi/internal/version"
)

func main() {
	var (
		noCache bool
		app     *App
	)

	root := &cobra.Command{
		Use:           "lakshmi",
		Short:         "Grounded thinking partner for the Indian investor",
		SilenceUsage:  true,
		SilenceErrors: true,
		// Build the shared App once, after global flags are parsed but
		// before any subcommand runs.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			a, err := buildApp(!noCache)
			if err != nil {
				return err
			}
			app = a
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runREPL(app)
		},
	}

	root.PersistentFlags().BoolVar(&noCache, "no-cache", false, "disable the local disk cache (also: LAKSHMI_NO_CACHE=1)")

	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "print the build version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println(version.Version)
			return nil
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "repl",
		Short: "launch the interactive shell",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runREPL(app)
		},
	})

	root.AddCommand(newLoginCmd())
	root.AddCommand(newLogoutCmd())
	root.AddCommand(newSessionCmd())
	root.AddCommand(newPortfolioCmd(&app))
	root.AddCommand(newCacheCmd(&app))
	root.AddCommand(newAskCmd(&app))

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func runREPL(app *App) error {
	if err := paths.EnsureHome(); err != nil {
		return fmt.Errorf("prepare home dir: %w", err)
	}
	hist, err := repl.NewHistory(paths.HistoryFile())
	if err != nil {
		return fmt.Errorf("load history: %w", err)
	}
	disp := repl.NewDispatcher()
	registerBrokerHandlers(disp)
	registerPortfolioHandlers(disp, app)
	registerCacheHandlers(disp, app)
	registerAskHandlers(disp, app)
	registerMoreHandler(disp, app)

	// Bare-integer shortcuts: typing `1` runs the first action listed
	// in the most recent answer's `📎 next:` line. Resolver consults
	// the live App.LastAnswer so it always reflects what's on screen.
	disp.SetShortcutResolver(func(n int) (string, bool) {
		ans := app.LastAnswer()
		if ans == nil || n < 1 || n > len(ans.NextActions) {
			return "", false
		}
		return ans.NextActions[n-1], true
	})

	model := repl.New(repl.Options{
		Banner:     repl.Banner(version.Version),
		History:    hist,
		Dispatcher: disp,
	})
	return repl.Run(model)
}
