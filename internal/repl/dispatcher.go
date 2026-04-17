package repl

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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

// SetFallback replaces the free-form-text handler.
func (d *Dispatcher) SetFallback(h Handler) {
	if h != nil {
		d.fallback = h
	}
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

func (d *Dispatcher) help(_ string) Response {
	var b strings.Builder
	b.WriteString("Available commands:\n")
	for _, c := range d.Commands() {
		b.WriteString("  ")
		b.WriteString(c.Name)
		b.WriteString(strings.Repeat(" ", max(2, 14-len(c.Name))))
		b.WriteString(c.Summary)
		b.WriteString("\n")
	}
	return Response{Output: b.String()}
}

func exitCommand(_ string) Response {
	return Response{Output: "Goodbye.\n", Quit: true}
}

func unknownCommand(input string) Response {
	return Response{
		Output: "unknown command: " + input + " — try /help\n",
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
