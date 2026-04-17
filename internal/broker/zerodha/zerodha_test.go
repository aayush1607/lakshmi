package zerodha

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	kiteconnect "github.com/zerodha/gokiteconnect/v4"

	"github.com/aayush1607/lakshmi/internal/broker"
	"github.com/aayush1607/lakshmi/internal/config"
)

func itoa(i int) string                              { return strconv.Itoa(i) }
func decodeQueryValue(s string) (string, error)      { return url.QueryUnescape(s) }

// fakeKite is a minimal in-memory implementation of the kiteAPI contract.
type fakeKite struct {
	loginURL    string
	session     kiteconnect.UserSession
	genErr      error
	gotReqToken string
	gotSecret   string
	accessToken string
}

func (f *fakeKite) GetLoginURL() string { return f.loginURL }
func (f *fakeKite) GenerateSession(rt, sec string) (kiteconnect.UserSession, error) {
	f.gotReqToken = rt
	f.gotSecret = sec
	if f.genErr != nil {
		return kiteconnect.UserSession{}, f.genErr
	}
	return f.session, nil
}
func (f *fakeKite) SetAccessToken(t string) { f.accessToken = t }

// pickPort asks the kernel for a free port, then immediately releases it.
// There is a theoretical race window with something else binding it
// before Client.Login does, but for localhost tests it is reliable enough.
func pickPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func newTestClient(t *testing.T, cfg config.Broker, fk *fakeKite, openedURL *string) *Client {
	t.Helper()
	sessions := broker.NewSessionStore(filepath.Join(t.TempDir(), "session.json"))
	tokens := broker.NewMemoryTokenStore()

	opener := func(u string) error {
		*openedURL = u
		return nil
	}
	fixed := time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC) // 15:30 IST
	return New(cfg, sessions, tokens,
		WithKiteAPI(fk),
		WithBrowserOpener(opener),
		WithNow(func() time.Time { return fixed }),
	)
}

func TestLoginHappyPath(t *testing.T) {
	port := pickPort(t)
	cfg := config.Broker{APIKey: "K", APISecret: "S", RedirectPort: port}
	fk := &fakeKite{
		loginURL: "https://kite.zerodha.com/connect/login?api_key=K&v=3",
		session: kiteconnect.UserSession{
			UserID:      "ZK1234",
			AccessToken: "tok-123",
			UserProfile: kiteconnect.UserProfile{UserID: "ZK1234", UserName: "AAYUSH  "},
		},
	}
	var openedURL string
	c := newTestClient(t, cfg, fk, &openedURL)

	// Hit the callback concurrently after Login opens its listener.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Extract state from the URL the "browser" was asked to open.
		state := waitForState(t, &openedURL)
		u := "http://127.0.0.1:" + itoa(port) + "/lakshmi/callback?state=" + state + "&status=success&request_token=reqtok-42"
		resp, err := httpGet(u)
		if err != nil {
			t.Errorf("callback GET: %v", err)
			return
		}
		if resp.StatusCode != 200 {
			t.Errorf("callback status = %d, want 200", resp.StatusCode)
		}
	}()

	sess, err := c.Login(context.Background())
	wg.Wait()
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if sess.UserID != "ZK1234" || sess.UserName != "AAYUSH" {
		t.Fatalf("session mismatch: %+v", sess)
	}
	if fk.gotReqToken != "reqtok-42" || fk.gotSecret != "S" {
		t.Fatalf("GenerateSession called with (%q,%q)", fk.gotReqToken, fk.gotSecret)
	}
	if fk.accessToken != "tok-123" {
		t.Fatalf("SetAccessToken got %q", fk.accessToken)
	}
	// Session must now be loadable + active.
	got, err := c.Session()
	if err != nil {
		t.Fatalf("Session after login: %v", err)
	}
	if got.UserID != "ZK1234" || !got.Active(time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("Session not active: %+v", got)
	}
	if !strings.Contains(openedURL, "redirect_params=") {
		t.Fatalf("login URL missing redirect_params: %s", openedURL)
	}
}

func TestLoginBadState(t *testing.T) {
	port := pickPort(t)
	cfg := config.Broker{APIKey: "K", APISecret: "S", RedirectPort: port}
	fk := &fakeKite{loginURL: "https://kite.zerodha.com/connect/login?api_key=K&v=3"}
	var openedURL string
	c := newTestClient(t, cfg, fk, &openedURL)

	go func() {
		_ = waitForState(t, &openedURL)
		// Use a WRONG state value.
		u := "http://127.0.0.1:" + itoa(port) + "/lakshmi/callback?state=wrong&request_token=reqtok-42&status=success"
		_, _ = httpGet(u)
	}()

	_, err := c.Login(context.Background())
	if err == nil || !strings.Contains(err.Error(), "invalid state") {
		t.Fatalf("expected invalid state error, got %v", err)
	}
}

func TestLoginKiteStatusError(t *testing.T) {
	port := pickPort(t)
	cfg := config.Broker{APIKey: "K", APISecret: "S", RedirectPort: port}
	fk := &fakeKite{loginURL: "https://kite.zerodha.com/connect/login?api_key=K&v=3"}
	var openedURL string
	c := newTestClient(t, cfg, fk, &openedURL)

	go func() {
		state := waitForState(t, &openedURL)
		u := "http://127.0.0.1:" + itoa(port) + "/lakshmi/callback?state=" + state + "&status=error&message=user+cancelled"
		_, _ = httpGet(u)
	}()

	_, err := c.Login(context.Background())
	if err == nil || !strings.Contains(err.Error(), "user cancelled") {
		t.Fatalf("expected kite status error, got %v", err)
	}
}

func TestLoginCtxCancelled(t *testing.T) {
	port := pickPort(t)
	cfg := config.Broker{APIKey: "K", APISecret: "S", RedirectPort: port}
	fk := &fakeKite{loginURL: "https://kite.zerodha.com/connect/login?api_key=K&v=3"}
	var openedURL string
	c := newTestClient(t, cfg, fk, &openedURL)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		// Wait for the browser to "open" then cancel before any callback.
		waitForState(t, &openedURL)
		cancel()
	}()
	_, err := c.Login(ctx)
	if !errors.Is(err, broker.ErrLoginCancelled) {
		t.Fatalf("want ErrLoginCancelled, got %v", err)
	}
}

func TestLoginGenerateSessionError(t *testing.T) {
	port := pickPort(t)
	cfg := config.Broker{APIKey: "K", APISecret: "S", RedirectPort: port}
	fk := &fakeKite{
		loginURL: "https://kite.zerodha.com/connect/login?api_key=K&v=3",
		genErr:   errors.New("boom"),
	}
	var openedURL string
	c := newTestClient(t, cfg, fk, &openedURL)

	go func() {
		state := waitForState(t, &openedURL)
		u := "http://127.0.0.1:" + itoa(port) + "/lakshmi/callback?state=" + state + "&status=success&request_token=R"
		_, _ = httpGet(u)
	}()

	_, err := c.Login(context.Background())
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("want boom, got %v", err)
	}
}

func TestLogoutClearsTokenAndSession(t *testing.T) {
	port := pickPort(t)
	cfg := config.Broker{APIKey: "K", APISecret: "S", RedirectPort: port}
	fk := &fakeKite{
		loginURL: "https://kite.zerodha.com/connect/login?api_key=K&v=3",
		session: kiteconnect.UserSession{
			UserID:      "Z1",
			AccessToken: "tok",
			UserProfile: kiteconnect.UserProfile{UserID: "Z1", UserName: "X"},
		},
	}
	var openedURL string
	c := newTestClient(t, cfg, fk, &openedURL)

	go func() {
		state := waitForState(t, &openedURL)
		u := "http://127.0.0.1:" + itoa(port) + "/lakshmi/callback?state=" + state + "&status=success&request_token=R"
		_, _ = httpGet(u)
	}()
	if _, err := c.Login(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := c.Logout(); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := c.Session(); !errors.Is(err, broker.ErrNotLoggedIn) {
		t.Fatalf("Session after Logout: %v", err)
	}
	// Logout is idempotent.
	if err := c.Logout(); err != nil {
		t.Fatalf("double Logout: %v", err)
	}
}

func TestNextKiteExpiry(t *testing.T) {
	ist := time.FixedZone("IST", 5*3600+30*60)
	cases := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{
			"pre-6am IST",
			time.Date(2026, 4, 17, 3, 0, 0, 0, ist),
			time.Date(2026, 4, 17, 6, 0, 0, 0, ist).UTC(),
		},
		{
			"post-6am IST",
			time.Date(2026, 4, 17, 10, 0, 0, 0, ist),
			time.Date(2026, 4, 18, 6, 0, 0, 0, ist).UTC(),
		},
		{
			"exactly 6am IST",
			time.Date(2026, 4, 17, 6, 0, 0, 0, ist),
			time.Date(2026, 4, 18, 6, 0, 0, 0, ist).UTC(),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := NextKiteExpiry(c.now)
			if !got.Equal(c.want) {
				t.Fatalf("got %s, want %s", got, c.want)
			}
		})
	}
}

// --- helpers ---

func waitForState(t *testing.T, opened *string) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		u := *opened
		if u == "" {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		i := strings.Index(u, "redirect_params=")
		if i < 0 {
			t.Fatalf("no redirect_params in %s", u)
		}
		// redirect_params is url-encoded "state=<hex>"
		enc := u[i+len("redirect_params="):]
		if amp := strings.Index(enc, "&"); amp >= 0 {
			enc = enc[:amp]
		}
		// Decode twice: once from the outer query, once to read state=.
		dec, err := decodeQueryValue(enc)
		if err != nil {
			t.Fatalf("decode redirect_params: %v", err)
		}
		if !strings.HasPrefix(dec, "state=") {
			t.Fatalf("redirect_params payload missing state=: %s", dec)
		}
		return strings.TrimPrefix(dec, "state=")
	}
	t.Fatal("browser was never asked to open")
	return ""
}

func httpGet(u string) (*http.Response, error) {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		return nil, err
	}
	// Drain so the server can finish the handler cleanly.
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp, nil
}
