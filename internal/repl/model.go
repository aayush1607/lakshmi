// Package repl implements the Lakshmi interactive shell (F1.1).
//
// The REPL is a Bubbletea program that:
//   - shows a banner on launch,
//   - reads a single input line at a time via a text input,
//   - dispatches slash-commands to registered handlers,
//   - routes everything else to a fallback handler (the agent, in later sprints),
//   - keeps a persistent history file navigable via Up/Down.
//
// Ctrl-C clears the current input line without exiting the shell.
// Ctrl-D and /exit quit cleanly.
package repl

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Options configures a new REPL model.
type Options struct {
	Banner     string
	History    *History
	Dispatcher *Dispatcher

	// Width/Height are initial dimensions used before Bubbletea sends a
	// WindowSizeMsg. Reasonable defaults are applied when zero.
	Width  int
	Height int
}

// Model is the Bubbletea model for the REPL.
type Model struct {
	opts     Options
	input    textinput.Model
	viewport viewport.Model
	history  *History
	disp     *Dispatcher

	ready       bool
	width       int
	height      int
	transcript  strings.Builder
	promptStyle lipgloss.Style
	userStyle   lipgloss.Style
	hintStyle   lipgloss.Style
}

// promptSymbol is the REPL prompt printed before every input.
// Rupee sign — unambiguously Indian, finance-themed, renders as a single
// clean glyph in every terminal (no emoji font quirks).
const promptSymbol = "₹ › "

// New builds a new Model. It does not start the program; call tea.NewProgram
// with it, or use Run for a convenience wrapper.
func New(opts Options) *Model {
	ti := textinput.New()
	ti.Prompt = promptSymbol
	ti.Placeholder = ""
	ti.CharLimit = 4096
	ti.Focus()

	vp := viewport.New(opts.Width, max(opts.Height-3, 10))

	m := &Model{
		opts:        opts,
		input:       ti,
		viewport:    vp,
		history:     opts.History,
		disp:        opts.Dispatcher,
		width:       opts.Width,
		height:      opts.Height,
		promptStyle: lipgloss.NewStyle().Bold(true),
		userStyle:   lipgloss.NewStyle().Faint(true),
		hintStyle:   lipgloss.NewStyle().Italic(true).Faint(true),
	}
	if opts.Banner != "" {
		m.transcript.WriteString(opts.Banner)
		m.transcript.WriteString("\n")
	}
	m.viewport.SetContent(m.transcript.String())
	return m
}

// Init satisfies tea.Model.
func (m *Model) Init() tea.Cmd { return textinput.Blink }

// Update satisfies tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width
		m.viewport.Height = max(msg.Height-3, 3)
		m.input.Width = max(msg.Width-lipgloss.Width(promptSymbol)-1, 10)
		m.ready = true
		m.viewport.GotoBottom()
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {

		case tea.KeyCtrlD:
			// EOF-like quit (even on an empty line).
			return m, tea.Quit

		case tea.KeyCtrlC:
			// Clear the current line; do NOT quit the REPL.
			m.input.SetValue("")
			if m.history != nil {
				m.history.ResetCursor()
			}
			return m, nil

		case tea.KeyEnter:
			line := m.input.Value()
			m.input.SetValue("")
			if m.history != nil {
				m.history.ResetCursor()
			}
			return m.submit(line)

		case tea.KeyUp:
			if m.history != nil {
				if v, ok := m.history.Prev(); ok {
					m.input.SetValue(v)
					m.input.CursorEnd()
				}
			}
			return m, nil

		case tea.KeyDown:
			if m.history != nil {
				if v, ok := m.history.Next(); ok {
					m.input.SetValue(v)
					m.input.CursorEnd()
				}
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *Model) submit(line string) (tea.Model, tea.Cmd) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		m.appendTranscript(promptSymbol + "\n")
		return m, nil
	}

	// Echo the input back (dimmed).
	m.appendTranscript(promptSymbol + m.userStyle.Render(line) + "\n")

	// Append to history (best-effort; persistence errors surface as a hint).
	if m.history != nil {
		if err := m.history.Append(trimmed); err != nil {
			m.appendTranscript(m.hintStyle.Render(fmt.Sprintf("(history write failed: %v)\n", err)))
		}
	}

	// Dispatch and render.
	resp := m.disp.Dispatch(trimmed)
	if resp.Output != "" {
		out := resp.Output
		if !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
		m.appendTranscript(out + "\n")
	}
	if resp.Quit {
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) appendTranscript(s string) {
	m.transcript.WriteString(s)
	m.viewport.SetContent(m.transcript.String())
	m.viewport.GotoBottom()
}

// View satisfies tea.Model.
func (m *Model) View() string {
	if !m.ready {
		return m.transcript.String() + "\n" + m.input.View() + "\n"
	}
	return m.viewport.View() + "\n" + m.input.View()
}

// Run creates a Program for the given Model and blocks until exit.
// It is provided for convenience; tests construct Model directly.
func Run(m *Model) error {
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
