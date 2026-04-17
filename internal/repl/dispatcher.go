package repl

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aayush1607/lakshmi/internal/shaper"
)

// Response is what a Handler returns: text to print plus an optional
// instruction to quit the REPL, and an optional tea.Cmd to run after
// the handler's Output is rendered. Long-running work (login, holdings,
// LLM calls) lives behind Follow so the REPL stays responsive.
type Response struct {
	Output string
	Quit   bool
	Follow tea.Cmd
}

// Handler handles a single dispatched input line.
//
// The raw input (including the leading slash for slash-commands, but
// without the trailing newline) is passed in so handlers can parse
// their own arguments. Handlers should be fast or return asynchronously;
// in later sprints long-running work will be wrapped behind tea.Cmds.
type Handler func(input string) Response

// Command describes a registered slash-command for /help rendering.
type Command struct {
	Name    string // e.g. "/help"
	Summary string // one-line description
}

// Dispatcher routes input lines to handlers. It owns the canonical
// command registry used by /help.
type Dispatcher struct {
	handlers map[string]Handler
	commands []Command

	// fallback is called when no slash command matches. For Sprint 1 this
	// is the "unknown command" message; from F1.5 onwards it routes to the
	// agent loop for free-form text.
	fallback Handler

	// shortcut, if set, is consulted before any slash-command lookup
	// when the input is a bare positive integer like "1" or "2". The
	// returned string is then re-dispatched as if the user had typed
	// it. This powers the F1.6 numbered "📎 next: [1] /p --by pnl"
	// shortcut: the user types `1` and we run the cached action.
	shortcut func(n int) (string, bool)
}

// NewDispatcher builds a dispatcher and registers the built-in commands
// (/help, /exit). Callers can register more handlers via Register.
func NewDispatcher() *Dispatcher {
	d := &Dispatcher{
		handlers: make(map[string]Handler),
		fallback: unknownCommand,
	}
	d.Register(Command{Name: "/help", Summary: "list available commands"}, d.help)
	d.Register(Command{Name: "/exit", Summary: "quit the shell"}, exitCommand)
	// vim-style quit aliases for muscle memory.
	d.handlers[":q"] = exitCommand
	d.handlers[":quit"] = exitCommand
	return d
}

// Register adds or replaces a command handler.
func (d *Dispatcher) Register(cmd Command, h Handler) {
	if cmd.Name == "" || h == nil {
		return
	}
	if _, exists := d.handlers[cmd.Name]; !exists {
		d.commands = append(d.commands, cmd)
	}
	d.handlers[cmd.Name] = h
}

// RegisterAlias adds a handler under a name that is NOT shown in /help.
// Useful for short/vim-style aliases ("/p" -> "/portfolio") without
// cluttering the command listing.
func (d *Dispatcher) RegisterAlias(name string, h Handler) {
	if name == "" || h == nil {
		return
	}
	d.handlers[name] = h
}

// SetFallback replaces the free-form-text handler.
func (d *Dispatcher) SetFallback(h Handler) {
	if h != nil {
		d.fallback = h
	}
}

// SetShortcutResolver wires the bare-integer shortcut. resolver(n)
// should return the command string to run for the user's "n", or
// (_, false) if there's no shortcut bound to that index right now.
// Pass nil to disable.
func (d *Dispatcher) SetShortcutResolver(resolver func(n int) (string, bool)) {
	d.shortcut = resolver
}

// Commands returns the registered commands sorted by name.
func (d *Dispatcher) Commands() []Command {
	out := make([]Command, len(d.commands))
	copy(out, d.commands)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Dispatch routes a single input line to the appropriate handler.
// Whitespace-only input yields a zero Response (caller should treat as no-op).
func (d *Dispatcher) Dispatch(input string) Response {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return Response{}
	}
	// Bare integer? Try the shortcut resolver first. We deliberately
	// check this BEFORE handler lookup so the user can't accidentally
	// shadow a number with a command (none exist today, but the rule
	// keeps the shortcut predictable).
	if d.shortcut != nil {
		if n, ok := parsePositiveInt(trimmed); ok {
			if cmd, ok := d.shortcut(n); ok {
				return d.Dispatch(cmd)
			}
		}
	}
	// Extract the first word to look up the handler; args are the rest.
	name := trimmed
	if i := strings.IndexAny(trimmed, " \t"); i >= 0 {
		name = trimmed[:i]
	}
	if h, ok := d.handlers[name]; ok {
		return h(trimmed)
	}
	if strings.HasPrefix(trimmed, "/") {
		return unknownCommand(trimmed)
	}
	return d.fallback(trimmed)
}

// parsePositiveInt returns (n, true) when s is a positive base-10 int
// in the range [1, 99]. We cap at 99 so a user pasting a large number
// (e.g. an order ID) doesn't accidentally hit the shortcut path.
func parsePositiveInt(s string) (int, bool) {
	if s == "" || len(s) > 2 {
		return 0, false
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	if n < 1 {
		return 0, false
	}
	return n, true
}

func (d *Dispatcher) help(_ string) Response {
	var b strings.Builder
	b.WriteString(StyleAccent.Render("Available commands:") + "\n")
	for _, c := range d.Commands() {
		b.WriteString("  ")
		b.WriteString(StylePrompt.Render(c.Name))
		b.WriteString(strings.Repeat(" ", max(2, 14-len(c.Name))))
		b.WriteString(StyleHint.Render(c.Summary))
		b.WriteString("\n")
	}
	return Response{Output: b.String()}
}

func exitCommand(_ string) Response {
	return Response{Output: StyleAccent.Render("Goodbye.") + "\n", Quit: true}
}

func unknownCommand(input string) Response {
	// Render through the shaper so unknown-commands look like every
	// other answer (yellow verdict + a /help next-action). Keeps the
	// "every answer is one shape" promise from F1.6.
	ans := shaper.Answer{
		Kind:        shaper.KindUnknown,
		Verdict:     shaper.VerdictYellow,
		VerdictText: "Unknown command: " + input,
		NextActions: []string{"/help"},
	}
	return Response{Output: shaper.Render(ans, shaper.DefaultTheme(), false)}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
