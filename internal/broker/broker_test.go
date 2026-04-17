package broker

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSessionStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	s := NewSessionStore(path)

	// Load on missing file returns ErrNotLoggedIn, not a hard error.
	if _, err := s.Load(); err != ErrNotLoggedIn {
		t.Fatalf("Load on empty: got %v, want ErrNotLoggedIn", err)
	}

	sess := Session{
		Provider:  ProviderZerodha,
		UserID:    "ZK1234",
		UserName:  "AAYUSH",
		Expiry:    time.Date(2026, 4, 18, 6, 0, 0, 0, time.UTC),
		FetchedAt: time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC),
	}
	if err := s.Save(sess); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != sess {
		t.Fatalf("roundtrip mismatch:\ngot:  %+v\nwant: %+v", got, sess)
	}

	if err := s.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, err := s.Load(); err != ErrNotLoggedIn {
		t.Fatalf("after Clear: %v", err)
	}
	// Clear on missing file is also fine.
	if err := s.Clear(); err != nil {
		t.Fatalf("Clear on missing: %v", err)
	}
}

func TestSessionActive(t *testing.T) {
	now := time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		sess Session
		want bool
	}{
		{"zero", Session{}, false},
		{"no user", Session{Provider: ProviderZerodha, Expiry: now.Add(time.Hour)}, false},
		{"expired", Session{Provider: ProviderZerodha, UserID: "Z1", Expiry: now.Add(-time.Minute)}, false},
		{"active", Session{Provider: ProviderZerodha, UserID: "Z1", Expiry: now.Add(time.Hour)}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.sess.Active(now); got != c.want {
				t.Fatalf("Active = %v, want %v", got, c.want)
			}
		})
	}
}

func TestMemoryTokenStore(t *testing.T) {
	ts := NewMemoryTokenStore()

	if _, err := ts.Get("zerodha"); err != ErrNoToken {
		t.Fatalf("Get empty: %v", err)
	}
	if err := ts.Set("zerodha", ""); err == nil {
		t.Fatal("Set empty must fail")
	}
	if err := ts.Set("zerodha", "abc123"); err != nil {
		t.Fatal(err)
	}
	got, err := ts.Get("zerodha")
	if err != nil || got != "abc123" {
		t.Fatalf("Get = (%q,%v)", got, err)
	}
	if err := ts.Delete("zerodha"); err != nil {
		t.Fatal(err)
	}
	if _, err := ts.Get("zerodha"); err != ErrNoToken {
		t.Fatalf("Get after Delete: %v", err)
	}
	// Delete on missing must not error.
	if err := ts.Delete("zerodha"); err != nil {
		t.Fatal(err)
	}
}
