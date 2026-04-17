// Package repl implements the Lakshmi interactive shell (F1.1).
//
// The REPL is a Bubbletea program that runs in **inline mode** (no alt
// screen, no mouse capture). Output lines are printed directly to the
// terminal scrollback via tea.Println; only the input line and the
// optional spinner footer are part of the live View. This means:
//
//   - native mouse-wheel scrolling works,
//   - native click-drag text selection / copy / paste works,
//   - the user's terminal scrollback contains the full session,
//
// without any modifier-key dance. Same architecture as `claude`,
// `gh copilot`, `psql`, and `python -i`.
//
// Ctrl-C clears the current input line without exiting the shell.
// Ctrl-D and /exit quit cleanly.
package repl

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
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
	opts    Options
	input   textinput.Model
	spinner spinner.Model
	history *History
	disp    *Dispatcher

	width       int
	height      int
	bannerShown bool
	promptStyle lipgloss.Style
	userStyle   lipgloss.Style
	hintStyle   lipgloss.Style

	// thinking is true while a Follow tea.Cmd is in flight. The footer
	// renders a spinner + cycling phrase so the shell doesn't look frozen
	// during a 2-5s LLM call. Cleared on the next TranscriptMsg.
	thinking      bool
	thinkPhrases  []string
	thinkPhraseAt int
	thinkStarted  time.Time
}

// promptSymbol is the REPL prompt printed before every input.
// Rupee sign — unambiguously Indian, finance-themed, renders as a single
// clean glyph in every terminal (no emoji font quirks).
const promptSymbol = "₹ › "

// thinkPhrases cycle while a long-running command (typically /ask) is
// in flight. They're deliberately on-brand and a little playful — the
// goal is to make the wait feel intentional, not laggy. Picked at
// random on each tick so re-runs don't always show the same line.
var defaultThinkPhrases = []string{
	"crunching numbers…",
	"reading your portfolio…",
	"checking the markets…",
	"asking the model…",
	"grounding the answer…",
	"counting your zeroes…",
	"weighing the evidence…",
	"finding the citations…",
	"opening the bhavcopy…",
	"thinking like a CA…",
}

// thinkPhraseInterval is how often the cycling phrase rotates while
// the spinner is active. Faster than the spinner tick to stay lively
// without distracting from the answer's eventual arrival.
const thinkPhraseInterval = 1500 * time.Millisecond

// thinkPhraseTickMsg drives the cycling phrase rotation while thinking.
type thinkPhraseTickMsg time.Time

func thinkPhraseTick() tea.Cmd {
	return tea.Tick(thinkPhraseInterval, func(t time.Time) tea.Msg { return thinkPhraseTickMsg(t) })
}

// TranscriptMsg asks the REPL to append a line to the transcript from an
// asynchronous tea.Cmd (e.g. a completed login flow). The trailing newline
// is added automatically if missing.
type TranscriptMsg struct {
	Text string
}

// New builds a new Model. It does not start the program; call tea.NewProgram
// with it, or use Run for a convenience wrapper.
func New(opts Options) *Model {
	ti := textinput.New()
	ti.Prompt = StylePrompt.Render(promptSymbol)
	ti.Placeholder = ""
	ti.PlaceholderStyle = StyleHint
	ti.PromptStyle = StylePrompt
	ti.TextStyle = lipgloss.NewStyle()
	ti.CharLimit = 4096
	ti.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = StyleSpinner

	w := opts.Width
	if w == 0 {
		w = 80
	}
	m := &Model{
		opts:         opts,
		input:        ti,
		spinner:      sp,
		history:      opts.History,
		disp:         opts.Dispatcher,
		width:        w,
		height:       opts.Height,
		promptStyle:  StylePrompt,
		userStyle:    StyleEcho,
		hintStyle:    StyleHint,
		thinkPhrases: defaultThinkPhrases,
	}
	m.input.Width = max(w-lipgloss.Width(promptSymbol)-1, 10)
	return m
}

// Init satisfies tea.Model. We print the banner once on first frame so
// it appears in the terminal scrollback (not as part of the live View).
func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{textinput.Blink}
	if m.opts.Banner != "" && !m.bannerShown {
		m.bannerShown = true
		cmds = append(cmds, tea.Println(m.opts.Banner))
	}
	return tea.Batch(cmds...)
}

// printCmd wraps tea.Println so transcript lines flow into the real
// terminal scrollback. Soft-wraps the text to current width first so
// long lines (e.g. agent answers) wrap predictably regardless of the
// terminal's own wrap behaviour.
func (m *Model) printCmd(s string) tea.Cmd {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return tea.Println("")
	}
	return tea.Println(wrapText(s, m.width))
}

// Update satisfies tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case TranscriptMsg:
		// Result has arrived — stop the spinner footer.
		m.thinking = false
		return m, m.printCmd(msg.Text)

	case spinner.TickMsg:
		// Only keep ticking while we're thinking; otherwise drop the
		// message so it doesn't keep the runtime busy.
		if !m.thinking {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case thinkPhraseTickMsg:
		if !m.thinking {
			return m, nil
		}
		// Pick a different random phrase than the current one.
		if len(m.thinkPhrases) > 1 {
			next := rand.Intn(len(m.thinkPhrases))
			if next == m.thinkPhraseAt {
				next = (next + 1) % len(m.thinkPhrases)
			}
			m.thinkPhraseAt = next
		}
		return m, thinkPhraseTick()

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.Width = max(msg.Width-lipgloss.Width(promptSymbol)-1, 10)
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
	echo := m.printCmd(StylePrompt.Render(promptSymbol) + StyleEcho.Render(line))

	if trimmed == "" {
		return m, echo
	}

	cmds := []tea.Cmd{echo}

	// Append to history (best-effort; persistence errors surface as a hint).
	if m.history != nil {
		if err := m.history.Append(trimmed); err != nil {
			cmds = append(cmds, m.printCmd(m.hintStyle.Render(fmt.Sprintf("(history write failed: %v)", err))))
		}
	}

	// Dispatch and render.
	resp := m.disp.Dispatch(trimmed)
	if resp.Output != "" {
		cmds = append(cmds, m.printCmd(resp.Output))
	}
	if resp.Quit {
		cmds = append(cmds, tea.Quit)
		return m, tea.Sequence(cmds...)
	}
	if resp.Follow != nil {
		// Long-running command: light up the spinner footer until the
		// follow-up tea.Cmd produces its TranscriptMsg.
		m.thinking = true
		m.thinkStarted = time.Now()
		m.thinkPhraseAt = rand.Intn(len(m.thinkPhrases))
		cmds = append(cmds, resp.Follow, m.spinner.Tick, thinkPhraseTick())
	}
	return m, tea.Batch(cmds...)
}

// View satisfies tea.Model. Inline mode: only the input (and an
// optional thinking footer) is the live render area. Everything else
// has already been printed to scrollback via tea.Println.
func (m *Model) View() string {
	if m.thinking {
		return m.thinkingFooter() + "\n" + m.input.View()
	}
	return m.input.View()
}

// thinkingFooter is the single line shown while a Follow tea.Cmd is in
// flight. Format: "<spinner> <phrase> · 2.3s". The elapsed time helps
// the user judge whether to wait or hit Ctrl-C.
func (m *Model) thinkingFooter() string {
	phrase := ""
	if m.thinkPhraseAt < len(m.thinkPhrases) {
		phrase = m.thinkPhrases[m.thinkPhraseAt]
	}
	elapsed := time.Since(m.thinkStarted).Round(100 * time.Millisecond)
	return fmt.Sprintf("%s %s %s",
		m.spinner.View(),
		StyleThink.Render(phrase),
		StyleTimer.Render("· "+elapsed.String()),
	)
}

// Run creates a Program for the given Model and blocks until exit.
// It is provided for convenience; tests construct Model directly.
//
// No alt-screen, no mouse capture. The REPL behaves like a normal
// shell: terminal scrollback shows the full session, mouse wheel and
// click-drag selection / copy / paste all work natively.
func Run(m *Model) error {
	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}
