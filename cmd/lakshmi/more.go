// Package main — `/more` slash command.
//
// Re-renders the most recent shaper.Answer with Detail expanded. The
// previous answer is stashed in App.SetLastAnswer by every handler
// that produces one (/portfolio, /ask, free-form text). Without a
// recent answer, /more prints a friendly hint.
//
// Implementation deliberately stateless beyond the one-pointer cache:
// `more` is read-only re-rendering, never re-runs tools or the LLM.
package main

import (
	"github.com/aayush1607/lakshmi/internal/repl"
	"github.com/aayush1607/lakshmi/internal/shaper"
)

func registerMoreHandler(disp *repl.Dispatcher, app *App) {
	h := func(_ string) repl.Response {
		ans := app.LastAnswer()
		if ans == nil {
			return repl.Response{
				Output: "  (nothing to expand — run /portfolio or /ask first)\n",
			}
		}
		return repl.Response{Output: shaper.Render(*ans, shaper.DefaultTheme(), true)}
	}
	disp.Register(repl.Command{
		Name:    "/more",
		Summary: "expand the full detail of the last answer",
	}, h)
	// Bare `more` — F1.6 spec uses it as the muscle-memory shortcut
	// (matches less/more pager convention).
	disp.RegisterAlias("more", h)
}
