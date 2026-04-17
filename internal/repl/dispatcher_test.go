package repl

import (
	"strings"
	"testing"
)

func TestDispatcherBuiltInsRegistered(t *testing.T) {
	d := NewDispatcher()
	names := []string{}
	for _, c := range d.Commands() {
		names = append(names, c.Name)
	}
	for _, want := range []string{"/help", "/exit"} {
		found := false
		for _, n := range names {
			if n == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing built-in command %s; have %v", want, names)
		}
	}
}

func TestDispatcherHelpListsRegisteredCommands(t *testing.T) {
	d := NewDispatcher()
	d.Register(Command{Name: "/portfolio", Summary: "show holdings"}, func(string) Response {
		return Response{Output: "ok"}
	})

	r := d.Dispatch("/help")
	if r.Quit {
		t.Fatal("/help must not quit")
	}
	for _, must := range []string{"/help", "/exit", "/portfolio", "show holdings"} {
		if !strings.Contains(r.Output, must) {
			t.Errorf("/help output missing %q:\n%s", must, r.Output)
		}
	}
}

func TestDispatcherExitSignalsQuit(t *testing.T) {
	d := NewDispatcher()
	r := d.Dispatch("/exit")
	if !r.Quit {
		t.Fatal("/exit must set Quit=true")
	}
}

func TestDispatcherVimStyleQuitAliases(t *testing.T) {
	d := NewDispatcher()
	for _, input := range []string{":q", ":quit"} {
		r := d.Dispatch(input)
		if !r.Quit {
			t.Errorf("%q must set Quit=true", input)
		}
	}
}

func TestDispatcherUnknownSlashCommand(t *testing.T) {
	d := NewDispatcher()
	r := d.Dispatch("/bogus")
	if r.Quit {
		t.Fatal("unknown command must not quit")
	}
	if !strings.Contains(r.Output, "unknown command") {
		t.Fatalf("expected 'unknown command' message, got: %s", r.Output)
	}
}

func TestDispatcherFallbackHandlesFreeText(t *testing.T) {
	d := NewDispatcher()
	called := false
	d.SetFallback(func(s string) Response {
		called = true
		if s != "what is my IT exposure?" {
			t.Errorf("unexpected fallback input: %q", s)
		}
		return Response{Output: "routed\n"}
	})
	r := d.Dispatch("  what is my IT exposure?  ")
	if !called {
		t.Fatal("fallback not invoked for free-form text")
	}
	if r.Output != "routed\n" {
		t.Fatalf("fallback output = %q", r.Output)
	}
}

func TestDispatcherEmptyInputIsNoOp(t *testing.T) {
	d := NewDispatcher()
	r := d.Dispatch("   ")
	if r.Output != "" || r.Quit || r.Follow != nil {
		t.Fatalf("expected zero Response for empty input, got %+v", r)
	}
}

func TestDispatcherSlashCommandWithArgs(t *testing.T) {
	d := NewDispatcher()
	var gotArg string
	d.Register(Command{Name: "/login", Summary: "log in"}, func(s string) Response {
		gotArg = s
		return Response{Output: "ok"}
	})

	r := d.Dispatch("/login zerodha")
	if r.Output != "ok" {
		t.Fatalf("handler not matched; got %+v", r)
	}
	if gotArg != "/login zerodha" {
		t.Fatalf("handler received %q, want full line", gotArg)
	}
}
