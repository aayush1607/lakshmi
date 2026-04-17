// Command lakshmi is the entry point for the Lakshmi CLI and interactive
// shell. In Sprint 1 it supports two subcommands:
//
//	lakshmi            launch the interactive REPL (default)
//	lakshmi version    print build version and exit
//
// Future sprints will attach login, portfolio, ta, fa, and other commands.
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
	root := &cobra.Command{
		Use:           "lakshmi",
		Short:         "Grounded thinking partner for the Indian investor",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runREPL()
		},
	}

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
			return runREPL()
		},
	})

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func runREPL() error {
	if err := paths.EnsureHome(); err != nil {
		return fmt.Errorf("prepare home dir: %w", err)
	}
	hist, err := repl.NewHistory(paths.HistoryFile())
	if err != nil {
		return fmt.Errorf("load history: %w", err)
	}
	disp := repl.NewDispatcher()
	disp.SetFallback(func(input string) repl.Response {
		// Sprint 1 stub. F1.5 replaces this with the agent loop.
		return repl.Response{
			Output: "free-form ask is not wired yet — coming in F1.5.\n",
		}
	})

	model := repl.New(repl.Options{
		Banner:     repl.Banner(version.Version),
		History:    hist,
		Dispatcher: disp,
	})
	return repl.Run(model)
}
