// Package main — agent wiring for F1.5 "grounded ask".
//
// This file is the shim between the Agent (pure, injectable) and the
// REPL / cobra surface (stateful, side-effectful). It owns:
//
//   - newAgent: builds an Agent from env-driven LLM config + the
//     process-wide cache, wiring the three Sprint 1 tools.
//   - askCmd:   the `lakshmi ask …` one-shot cobra subcommand.
//   - registerAskHandlers: wires `/ask`, `/why`, and the free-form
//     fallback into the REPL with per-session conversation history.
package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/aayush1607/lakshmi/internal/agent"
	"github.com/aayush1607/lakshmi/internal/config"
	"github.com/aayush1607/lakshmi/internal/llm"
	"github.com/aayush1607/lakshmi/internal/repl"
	"github.com/aayush1607/lakshmi/internal/tools"
)

// askTimeout bounds a single end-to-end Run call. 30s is comfortably
// above the 10s target in the spec so we don't chop responses when the
// network is slow, but low enough that a wedged LLM eventually gives up.
const askTimeout = 30 * time.Second

// historyCap limits how many prior (user, assistant) message pairs we
// send with each question. Keeps token usage bounded on long sessions.
// 3 pairs = 6 messages, which comfortably meets the spec's "3 follow-up
// questions" requirement.
const historyCap = 3

// newAgent builds a fully-wired Agent. Returns nil plus the config
// error when Azure Foundry env vars are missing — the REPL handles
// that case by refusing free-form input with a helpful message.
func newAgent(app *App) (*agent.Agent, error) {
	cfg, err := config.LoadLLM()
	if err != nil {
		return nil, err
	}
	client := llm.NewAzure(llm.AzureConfig{
		Endpoint:   cfg.Endpoint,
		Deployment: cfg.Deployment,
		APIKey:     cfg.APIKey,
		APIVersion: cfg.APIVersion,
	})

	// Tools: each is built lazily in its Call method. The broker is
	// constructed *inside* the tool so we don't crash at agent build
	// time when the user hasn't logged in yet — login errors surface
	// only when they ask a portfolio question.
	reg := tools.NewRegistry()
	reg.Register(tools.NewTimeNowTool())
	reg.Register(tools.NewSectorLookupTool())
	reg.Register(&lazyHoldings{app: app})

	return agent.New(agent.Options{LLM: client, Tools: reg}), nil
}

// lazyHoldings wraps tools.HoldingsTool so the broker is only resolved
// when the agent actually calls it. This lets `lakshmi` start without
// KITE_* env vars as long as the user never asks a portfolio question.
type lazyHoldings struct {
	app *App
}

func (l *lazyHoldings) Name() string        { return "portfolio_holdings" }
func (l *lazyHoldings) Description() string { return "User's Zerodha equity holdings." }
func (l *lazyHoldings) Call(ctx context.Context, args map[string]any) (tools.Result, error) {
	b, err := newBroker()
	if err != nil {
		return tools.Result{}, err
	}
	return tools.NewHoldingsTool(b, l.app.Cache).Call(ctx, args)
}

// newAskCmd wires the one-shot `lakshmi ask` subcommand.
func newAskCmd(appRef **App) *cobra.Command {
	return &cobra.Command{
		Use:   "ask [question...]",
		Short: "ask Lakshmi a grounded question about your portfolio",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := strings.TrimSpace(strings.Join(args, " "))
			if query == "" {
				return errors.New("question is empty")
			}
			a, err := newAgent(*appRef)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), askTimeout)
			defer cancel()
			ans, err := a.Run(ctx, query, nil)
			if err != nil {
				return err
			}
			fmt.Println(agent.Render(ans))
			return nil
		},
	}
}

// registerAskHandlers installs /ask, /why, and the free-form fallback.
// Conversation history and the last-answer-for-/why are captured in
// closures so they live for the REPL session and reset between runs.
func registerAskHandlers(disp *repl.Dispatcher, app *App) {
	// Lazy-built so a missing AZURE_FOUNDRY_* config doesn't crash the
	// REPL — only /ask fails, the rest of the shell keeps working.
	var (
		ag        *agent.Agent
		agErr     error
		agBuilt   bool
		history   []llm.Message
		lastTrace agent.Trace
		hasTrace  bool
	)
	ensureAgent := func() error {
		if agBuilt {
			return agErr
		}
		ag, agErr = newAgent(app)
		agBuilt = true
		return agErr
	}

	ask := func(input string) repl.Response {
		// Strip the slash command prefix if present.
		query := strings.TrimSpace(strings.TrimPrefix(input, "/ask"))
		if query == "" {
			return repl.Response{Output: "usage: /ask <question>\n"}
		}
		if err := ensureAgent(); err != nil {
			return repl.Response{Output: "  ✗ " + humanLLMErr(err) + "\n"}
		}
		// No inline "thinking…" — the REPL footer shows a live spinner
		// the whole time the Follow cmd is running.
		return repl.Response{
			Follow: askCmd(ag, query, history, &history, &lastTrace, &hasTrace, app),
		}
	}

	fallback := func(input string) repl.Response {
		// Free-form text. Same code path as /ask, just without the prefix.
		if err := ensureAgent(); err != nil {
			return repl.Response{Output: "  ✗ " + humanLLMErr(err) + "\n"}
		}
		return repl.Response{
			Follow: askCmd(ag, input, history, &history, &lastTrace, &hasTrace, app),
		}
	}

	why := func(_ string) repl.Response {
		if !hasTrace {
			return repl.Response{Output: "  (no recent answer — ask a question first)\n"}
		}
		return repl.Response{Output: lastTrace.String()}
	}

	disp.Register(repl.Command{Name: "/ask", Summary: "ask Lakshmi a grounded question"}, ask)
	disp.Register(repl.Command{Name: "/why", Summary: "show the reasoning trace of the last answer"}, why)
	disp.SetFallback(fallback)
}

// askCmd is the tea.Cmd that actually runs the agent. It appends to
// the history slice (via the pointer) and stashes the trace so /why
// can print it later. It also writes the rendered shaper.Answer into
// app.SetLastAnswer so /more can re-render it expanded.
func askCmd(
	ag *agent.Agent,
	query string,
	history []llm.Message,
	historyPtr *[]llm.Message,
	tracePtr *agent.Trace,
	hasTracePtr *bool,
	app *App,
) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), askTimeout)
		defer cancel()
		ans, err := ag.Run(ctx, query, history)
		if err != nil {
			return repl.TranscriptMsg{Text: "  ✗ " + err.Error()}
		}
		*tracePtr = ans.Trace
		*hasTracePtr = true
		// Append this turn to history for the next question. Truncate
		// to historyCap pairs to keep token usage bounded.
		*historyPtr = appendTurn(history, query, ans.Text)
		// Stash the universal-shape Answer so /more can expand it.
		shaped := agent.ToShaper(ans)
		app.SetLastAnswer(&shaped)
		return repl.TranscriptMsg{Text: agent.Render(ans)}
	}
}

// appendTurn adds (user, assistant) to history and trims to historyCap
// pairs. We keep the freshest turns — older context is rarely relevant
// once the user has moved on to a new topic.
func appendTurn(prior []llm.Message, query, answer string) []llm.Message {
	next := append([]llm.Message{}, prior...)
	next = append(next,
		llm.Message{Role: llm.RoleUser, Content: query},
		llm.Message{Role: llm.RoleAssistant, Content: answer},
	)
	maxMsgs := historyCap * 2
	if len(next) > maxMsgs {
		next = next[len(next)-maxMsgs:]
	}
	return next
}

// humanLLMErr rewrites config.ErrLLMNotConfigured into guidance the
// user can act on without spelunking the README.
func humanLLMErr(err error) string {
	if errors.Is(err, config.ErrLLMNotConfigured) {
		return "LLM is not configured. Export AZURE_FOUNDRY_ENDPOINT, AZURE_FOUNDRY_DEPLOYMENT, and AZURE_FOUNDRY_API_KEY."
	}
	return err.Error()
}
