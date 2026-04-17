// Package zerodha implements a Kite Connect (Zerodha) broker client.
//
// The OAuth-style login flow runs entirely on the user's machine:
//
//  1. Start a short-lived HTTP listener on 127.0.0.1:<RedirectPort>.
//  2. Open the browser at Kite's login URL (includes a random state nonce).
//  3. Wait for Kite to redirect to /lakshmi/callback with ?request_token=...
//  4. Validate the state, exchange the request_token for an access_token,
//     persist the token to the OS keychain, and the session metadata
//     (user id, expiry) to a JSON file.
//
// The browser opener and the Kite API client are both behind small
// interfaces so the flow can be end-to-end tested without touching the
// network or a real browser.
package zerodha

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"

	kiteconnect "github.com/zerodha/gokiteconnect/v4"

	"github.com/aayush1607/lakshmi/internal/broker"
	"github.com/aayush1607/lakshmi/internal/config"
)

// Config bundles the settings the Zerodha client needs. Secrets live in
// this struct only for the duration of a single Login call; they are
// never persisted.
type Config = config.Broker

// kiteAPI is the subset of gokiteconnect we actually call. It exists so
// tests can inject a fake session generator without making HTTP calls.
type kiteAPI interface {
	GetLoginURL() string
	GenerateSession(requestToken, apiSecret string) (kiteconnect.UserSession, error)
	SetAccessToken(token string)
}

// Option configures a Client.
type Option func(*Client)

// WithKiteAPI replaces the underlying Kite SDK client (used in tests).
func WithKiteAPI(k kiteAPI) Option { return func(c *Client) { c.kite = k } }

// WithBrowserOpener swaps the function that opens the system browser
// (used in tests to avoid launching a real browser).
func WithBrowserOpener(open func(string) error) Option {
	return func(c *Client) { c.openBrowser = open }
}

// WithNow replaces time.Now (used to make expiry computation deterministic
// in tests).
func WithNow(now func() time.Time) Option { return func(c *Client) { c.now = now } }

// Client is a read-only Broker implementation that talks to Zerodha Kite.
type Client struct {
	cfg         Config
	sessions    *broker.SessionStore
	tokens      broker.TokenStore
	kite        kiteAPI
	openBrowser func(string) error
	now         func() time.Time
}

// New builds a Client. The real gokiteconnect client is used by default;
// tests should inject via WithKiteAPI to avoid network calls.
func New(cfg Config, sessions *broker.SessionStore, tokens broker.TokenStore, opts ...Option) *Client {
	c := &Client{
		cfg:         cfg,
		sessions:    sessions,
		tokens:      tokens,
		kite:        kiteconnect.New(cfg.APIKey),
		openBrowser: openURL,
		now:         time.Now,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Provider returns broker.ProviderZerodha.
func (c *Client) Provider() broker.Provider { return broker.ProviderZerodha }

// Session returns the persisted session, or an error if the user is not
// logged in or the session has expired.
func (c *Client) Session() (broker.Session, error) {
	sess, err := c.sessions.Load()
	if err != nil {
		return broker.Session{}, err
	}
	if !sess.Active(c.now()) {
		return sess, broker.ErrSessionExpired
	}
	return sess, nil
}

// Logout clears both the token (keychain) and the session metadata (disk).
// It is idempotent: calling Logout when already logged out is a no-op.
func (c *Client) Logout() error {
	// Delete both; report the first real error. Missing-item errors are
	// swallowed by the stores so this is safe on a clean machine.
	if err := c.tokens.Delete(string(broker.ProviderZerodha)); err != nil {
		return fmt.Errorf("clear token: %w", err)
	}
	if err := c.sessions.Clear(); err != nil {
		return fmt.Errorf("clear session: %w", err)
	}
	return nil
}

// Holdings is implemented in F1.3; for F1.2 it returns ErrNotImplemented
// so the interface compiles and the error is explicit.
func (c *Client) Holdings(ctx context.Context) ([]broker.Holding, error) {
	return nil, broker.ErrNotImplemented
}

// Login runs the OAuth-style browser flow. It blocks until the user
// completes the flow, the ctx is cancelled, or the callback server
// returns an error.
//
// On success the access token is stored in the keychain, the session
// metadata (user id, expiry) is persisted to disk, and the Session is
// returned.
func (c *Client) Login(ctx context.Context) (broker.Session, error) {
	state, err := randomState()
	if err != nil {
		return broker.Session{}, fmt.Errorf("generate state: %w", err)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", c.cfg.RedirectPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return broker.Session{}, fmt.Errorf("listen on %s (is the redirect port busy?): %w", addr, err)
	}
	defer ln.Close()

	type callback struct {
		requestToken string
		err          error
	}
	done := make(chan callback, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/lakshmi/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		// Kite can also send action=login&status=error&message=...
		if s := q.Get("status"); s != "" && s != "success" {
			msg := q.Get("message")
			if msg == "" {
				msg = s
			}
			writeCallbackPage(w, false, msg)
			done <- callback{err: fmt.Errorf("kite login failed: %s", msg)}
			return
		}
		if got := q.Get("state"); got != state {
			writeCallbackPage(w, false, "invalid state")
			done <- callback{err: errors.New("invalid state in callback (possible CSRF)")}
			return
		}
		rt := q.Get("request_token")
		if rt == "" {
			writeCallbackPage(w, false, "missing request_token")
			done <- callback{err: errors.New("callback missing request_token")}
			return
		}
		writeCallbackPage(w, true, "")
		done <- callback{requestToken: rt}
	})

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go srv.Serve(ln) //nolint:errcheck // Serve returns on Close()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	loginURL := buildLoginURL(c.kite.GetLoginURL(), state)
	if err := c.openBrowser(loginURL); err != nil {
		// Non-fatal: print the URL so the user can paste it manually.
		return broker.Session{}, fmt.Errorf("open browser (paste this URL manually: %s): %w", loginURL, err)
	}

	var cb callback
	select {
	case cb = <-done:
	case <-ctx.Done():
		return broker.Session{}, broker.ErrLoginCancelled
	}
	if cb.err != nil {
		return broker.Session{}, cb.err
	}

	us, err := c.kite.GenerateSession(cb.requestToken, c.cfg.APISecret)
	if err != nil {
		return broker.Session{}, fmt.Errorf("kite generate-session: %w", err)
	}
	if us.AccessToken == "" || us.UserID == "" {
		return broker.Session{}, errors.New("kite returned an empty session")
	}
	c.kite.SetAccessToken(us.AccessToken)

	if err := c.tokens.Set(string(broker.ProviderZerodha), us.AccessToken); err != nil {
		return broker.Session{}, fmt.Errorf("store access token: %w", err)
	}
	now := c.now()
	sess := broker.Session{
		Provider:  broker.ProviderZerodha,
		UserID:    us.UserID,
		UserName:  strings.TrimSpace(us.UserName),
		Expiry:    NextKiteExpiry(now),
		FetchedAt: now.UTC(),
	}
	if sess.UserName == "" {
		sess.UserName = us.UserShortName
	}
	if err := c.sessions.Save(sess); err != nil {
		return broker.Session{}, fmt.Errorf("store session: %w", err)
	}
	return sess, nil
}

// NextKiteExpiry returns the next Kite access-token expiry boundary,
// which is 06:00 IST on the next calendar day (or today, if called
// before 06:00 IST).
func NextKiteExpiry(now time.Time) time.Time {
	ist := time.FixedZone("IST", 5*3600+30*60)
	n := now.In(ist)
	today6 := time.Date(n.Year(), n.Month(), n.Day(), 6, 0, 0, 0, ist)
	if n.Before(today6) {
		return today6.UTC()
	}
	return today6.AddDate(0, 0, 1).UTC()
}

func buildLoginURL(base, state string) string {
	u, err := url.Parse(base)
	if err != nil {
		// Extremely unlikely; fall back to appending.
		if strings.Contains(base, "?") {
			return base + "&redirect_params=" + url.QueryEscape("state="+state)
		}
		return base + "?redirect_params=" + url.QueryEscape("state="+state)
	}
	q := u.Query()
	// Kite echoes back any key/value pairs in redirect_params on the callback.
	q.Set("redirect_params", "state="+state)
	u.RawQuery = q.Encode()
	return u.String()
}

func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func openURL(u string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	return cmd.Start()
}

func writeCallbackPage(w http.ResponseWriter, ok bool, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	title := "Login complete"
	body := "You are logged in to Lakshmi. You may close this window."
	if !ok {
		title = "Login failed"
		body = "Lakshmi could not complete the login: " + message + ". You may close this window."
		w.WriteHeader(http.StatusBadRequest)
	}
	fmt.Fprintf(w, `<!doctype html>
<html><head><meta charset="utf-8"><title>%s</title>
<style>body{font:15px/1.5 -apple-system,Segoe UI,Roboto,sans-serif;max-width:32rem;margin:4rem auto;padding:0 1rem;color:#222}
h1{font-size:1.2rem;margin:0 0 .5rem}</style></head>
<body><h1>Lakshmi — %s</h1><p>%s</p></body></html>`, title, title, body)
}
