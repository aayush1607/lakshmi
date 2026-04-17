// Package main — broker wiring for the lakshmi CLI and REPL. This file
// builds a Zerodha broker and exposes the `login`/`logout`/`session`
// subcommands along with the matching REPL slash-command handlers.
package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/aayush1607/lakshmi/internal/broker"
	"github.com/aayush1607/lakshmi/internal/broker/zerodha"
	"github.com/aayush1607/lakshmi/internal/config"
	"github.com/aayush1607/lakshmi/internal/paths"
	"github.com/aayush1607/lakshmi/internal/repl"
)

// loginTimeout bounds how long we wait for the user to complete the
// browser flow. Five minutes is generous but prevents a hung listener.
const loginTimeout = 5 * time.Minute

// newBroker builds a Zerodha broker wired to the real keychain + session
// file. Returns a helpful error if credentials are not configured.
func newBroker() (broker.Broker, error) {
	cfg, err := config.LoadBroker()
	if err != nil {
		return nil, err
	}
	return zerodha.New(
		cfg,
		broker.NewSessionStore(paths.SessionFile()),
		broker.NewKeyringTokenStore(),
	), nil
}

// --- cobra subcommands ---

func newLoginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "log in to Zerodha via browser",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := paths.EnsureHome(); err != nil {
				return err
			}
			b, err := newBroker()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), loginTimeout)
			defer cancel()
			fmt.Println("Opening browser for Zerodha login…")
			sess, err := b.Login(ctx)
			if err != nil {
				return err
			}
			fmt.Println(formatLoginSuccess(sess))
			return nil
		},
	}
}

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "clear the stored Zerodha session and token",
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := newBroker()
			if err != nil {
				return err
			}
			if err := b.Logout(); err != nil {
				return err
			}
			fmt.Println("Logged out. Token and session cleared.")
			return nil
		},
	}
}

func newSessionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "session",
		Short: "show the current login status",
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := newBroker()
			if err != nil {
				// Credentials missing is a reportable state, but the user
				// still deserves a clean "not logged in" answer for session.
				fmt.Println("Not logged in (" + err.Error() + ").")
				return nil
			}
			sess, err := b.Session()
			fmt.Println(formatSessionStatus(sess, err))
			return nil
		},
	}
}

// --- REPL slash-commands ---

// registerBrokerHandlers wires /login, /logout, /login status (via
// argument parsing) onto the dispatcher. It tolerates an unconfigured
// broker: the handlers surface the configuration error rather than
// preventing the REPL from starting.
func registerBrokerHandlers(disp *repl.Dispatcher) {
	disp.Register(repl.Command{
		Name:    "/login",
		Summary: "log in to Zerodha — add 'status' to see current state",
	}, func(input string) repl.Response {
		parts := strings.Fields(input)
		// /login status or /login zerodha status
		if hasArg(parts, "status") {
			return sessionResponse()
		}
		return startLoginResponse()
	})

	disp.Register(repl.Command{
		Name:    "/logout",
		Summary: "clear the stored Zerodha session",
	}, func(input string) repl.Response {
		b, err := newBroker()
		if err != nil {
			return repl.Response{Output: "  ✗ " + err.Error() + "\n"}
		}
		if err := b.Logout(); err != nil {
			return repl.Response{Output: "  ✗ logout failed: " + err.Error() + "\n"}
		}
		return repl.Response{Output: "  ✓ logged out. Token and session cleared.\n"}
	})
}

// startLoginResponse kicks off an async Zerodha login. It returns
// immediately with a status line; the completion lands later via a
// TranscriptMsg emitted by the Follow cmd.
func startLoginResponse() repl.Response {
	b, err := newBroker()
	if err != nil {
		return repl.Response{Output: "  ✗ " + err.Error() + "\n"}
	}
	msg := "  ⟳ Opening browser for Zerodha login — complete the flow in your browser.\n" +
		"    (this shell stays responsive; you will see a confirmation here when done.)\n"
	return repl.Response{
		Output: msg,
		Follow: loginCmd(b),
	}
}

func loginCmd(b broker.Broker) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), loginTimeout)
		defer cancel()
		sess, err := b.Login(ctx)
		if err != nil {
			return repl.TranscriptMsg{Text: "  ✗ login failed: " + err.Error()}
		}
		return repl.TranscriptMsg{Text: formatLoginSuccess(sess)}
	}
}

// sessionResponse builds the REPL response for /login status. It is
// synchronous (no network calls) — just reads session.json and formats.
func sessionResponse() repl.Response {
	b, err := newBroker()
	if err != nil {
		return repl.Response{Output: "  Not logged in (" + err.Error() + ").\n"}
	}
	sess, err := b.Session()
	return repl.Response{Output: formatSessionStatus(sess, err) + "\n"}
}

func hasArg(parts []string, want string) bool {
	for _, p := range parts[1:] {
		if strings.EqualFold(p, want) {
			return true
		}
	}
	return false
}

func formatLoginSuccess(s broker.Session) string {
	return fmt.Sprintf("  ✓ Connected as %s (%s)\n    Session valid until %s.",
		defaultStr(s.UserName, s.UserID),
		s.UserID,
		formatExpiry(s.Expiry),
	)
}

func formatSessionStatus(s broker.Session, err error) string {
	if errors.Is(err, broker.ErrNotLoggedIn) {
		return "  Not logged in. Run /login or `lakshmi login`."
	}
	if errors.Is(err, broker.ErrSessionExpired) {
		return fmt.Sprintf("  Session expired for %s (%s). Run /login to refresh.",
			defaultStr(s.UserName, s.UserID), s.UserID)
	}
	if err != nil {
		return "  ✗ session error: " + err.Error()
	}
	return fmt.Sprintf("  ✓ Connected as %s (%s) · valid until %s.",
		defaultStr(s.UserName, s.UserID), s.UserID, formatExpiry(s.Expiry))
}

func formatExpiry(t time.Time) string {
	ist := time.FixedZone("IST", 5*3600+30*60)
	return t.In(ist).Format("02 Jan 2006, 15:04 IST")
}

func defaultStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
