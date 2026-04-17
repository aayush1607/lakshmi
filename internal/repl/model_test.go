package repl

import (
	"bytes"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

// newTestModel wires a Model with its own temp history file for isolation.
func newTestModel(t *testing.T, opts ...func(*Options)) *Model {
	t.Helper()
	h, err := NewHistory(t.TempDir() + "/history")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	o := Options{
		Banner:     Banner("test"),
		History:    h,
		Dispatcher: NewDispatcher(),
		Width:      80,
		Height:     24,
	}
	for _, fn := range opts {
		fn(&o)
	}
	return New(o)
}

func TestREPLShowsBannerOnStart(t *testing.T) {
	m := newTestModel(t)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		// Match either the ASCII-art logo or the version stamp — both are in the banner.
		return bytes.Contains(b, []byte("lakshmi")) || bytes.Contains(b, []byte("test"))
	}, teatest.WithCheckInterval(10*time.Millisecond), teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlD})
	if err := tm.Quit(); err != nil {
		t.Fatal(err)
	}
}

func TestREPLUnknownCommandDoesNotCrash(t *testing.T) {
	m := newTestModel(t)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	tm.Type("/bogus")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("unknown command"))
	}, teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlD})
	if err := tm.Quit(); err != nil {
		t.Fatal(err)
	}
}

func TestREPLHelpListsCommands(t *testing.T) {
	m := newTestModel(t)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	tm.Type("/help")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		s := string(b)
		return strings.Contains(s, "/help") && strings.Contains(s, "/exit")
	}, teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlD})
	if err := tm.Quit(); err != nil {
		t.Fatal(err)
	}
}

func TestREPLExitCommandQuitsCleanly(t *testing.T) {
	m := newTestModel(t)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	tm.Type("/exit")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

func TestREPLCtrlCClearsLineDoesNotExit(t *testing.T) {
	m := newTestModel(t)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	tm.Type("partial input")
	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})

	// After Ctrl-C, the model should still be alive and ready for /exit.
	tm.Type("/exit")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

func TestREPLCtrlDQuits(t *testing.T) {
	m := newTestModel(t)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlD})

	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}
