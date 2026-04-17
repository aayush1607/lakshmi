package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type holding struct {
	Symbol string  `json:"symbol"`
	Qty    int     `json:"qty"`
	LTP    float64 `json:"ltp"`
}

func TestPutGetRoundTrip(t *testing.T) {
	s, err := Open(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	want := []holding{{Symbol: "TCS", Qty: 15, LTP: 3980}}
	if err := s.Put(NSHoldings, "ZK1234", want); err != nil {
		t.Fatal(err)
	}
	entry, hit, err := s.Get(NSHoldings, "ZK1234")
	if err != nil || !hit {
		t.Fatalf("Get: hit=%v err=%v", hit, err)
	}
	var got []holding
	if err := json.Unmarshal(entry.Payload, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Symbol != "TCS" {
		t.Fatalf("payload: %+v", got)
	}
	if time.Since(entry.FetchedAt) > 5*time.Second {
		t.Errorf("FetchedAt too old: %v", entry.FetchedAt)
	}
	if entry.Age < 0 {
		t.Errorf("Age negative: %v", entry.Age)
	}
}

func TestGetMiss(t *testing.T) {
	s, _ := Open(t.TempDir(), true)
	_, hit, err := s.Get(NSHoldings, "none")
	if err != nil || hit {
		t.Fatalf("expected clean miss, hit=%v err=%v", hit, err)
	}
}

func TestDisabledStoreNeverTouchesDisk(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(NSHoldings, "ZK1234", []holding{{Symbol: "TCS"}}); err != nil {
		t.Fatal(err)
	}
	_, hit, _ := s.Get(NSHoldings, "ZK1234")
	if hit {
		t.Fatal("disabled store must not hit")
	}
	// Acceptance #6: no new files under the data dir in no-cache mode.
	var files []string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if len(files) > 0 {
		t.Fatalf("disabled store wrote files: %v", files)
	}
}

func TestSetEnabledToggles(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir, false)
	_ = s.Put(NSHoldings, "k", 1) // no-op
	if err := s.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	_ = s.Put(NSHoldings, "k", 2)
	_, hit, _ := s.Get(NSHoldings, "k")
	if !hit {
		t.Fatal("expected hit after SetEnabled(true)")
	}
	if err := s.SetEnabled(false); err != nil {
		t.Fatal(err)
	}
	_, hit, _ = s.Get(NSHoldings, "k")
	if hit {
		t.Fatal("disabled store must not hit even with existing file")
	}
}

func TestClear(t *testing.T) {
	s, _ := Open(t.TempDir(), true)
	_ = s.Put(NSHoldings, "a", 1)
	_ = s.Put(NSQuotes, "b", 2)
	if err := s.Clear(); err != nil {
		t.Fatal(err)
	}
	st, _ := s.Stats()
	if st.TotalBytes != 0 || len(st.Namespaces) != 0 {
		t.Fatalf("expected empty after Clear, got %+v", st)
	}
	// Clear on empty dir is idempotent.
	if err := s.Clear(); err != nil {
		t.Fatal(err)
	}
}

func TestStats(t *testing.T) {
	s, _ := Open(t.TempDir(), true)
	_ = s.Put(NSHoldings, "ZK1234", []holding{{Symbol: "TCS"}})
	_ = s.Put(NSQuotes, "RELIANCE", 1234.5)
	_ = s.Put(NSQuotes, "TCS", 3980.0)

	st, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if !st.Enabled {
		t.Error("Enabled should be true")
	}
	if len(st.Namespaces) != 2 {
		t.Fatalf("want 2 namespaces, got %d (%+v)", len(st.Namespaces), st.Namespaces)
	}
	var holds, quotes NamespaceStats
	for _, n := range st.Namespaces {
		if n.Namespace == NSHoldings {
			holds = n
		}
		if n.Namespace == NSQuotes {
			quotes = n
		}
	}
	if holds.Entries != 1 {
		t.Errorf("holdings entries = %d", holds.Entries)
	}
	if quotes.Entries != 2 {
		t.Errorf("quotes entries = %d", quotes.Entries)
	}
	if st.TotalBytes == 0 {
		t.Error("TotalBytes = 0")
	}
	if holds.LastRefresh.IsZero() {
		t.Error("LastRefresh not set")
	}
}

func TestStatsEmpty(t *testing.T) {
	s, _ := Open(t.TempDir(), true)
	st, err := s.Stats()
	if err != nil || len(st.Namespaces) != 0 || st.TotalBytes != 0 {
		t.Fatalf("empty stats: %+v err=%v", st, err)
	}
}

func TestFreshnessRulesHoldings(t *testing.T) {
	r := Rules[NSHoldings]
	if r(time.Minute, true) {
		t.Error("holdings should never be fresh in market hours")
	}
	if r(25*time.Hour, true) {
		t.Error("holdings stale in market hours always")
	}
	if !r(time.Hour, false) {
		t.Error("holdings should be fresh at 1h off-hours")
	}
	if r(25*time.Hour, false) {
		t.Error("holdings should be stale at 25h off-hours")
	}
}

func TestFreshnessRulesQuotes(t *testing.T) {
	r := Rules[NSQuotes]
	if r(time.Second, true) {
		t.Error("quotes never fresh in market hours")
	}
	if !r(72*time.Hour, false) {
		t.Error("quotes always fresh off-hours (cache == last close)")
	}
}

func TestFreshnessRulesFundamentals(t *testing.T) {
	r := Rules[NSFundamentals]
	if !r(10*time.Minute, true) || !r(10*time.Minute, false) {
		t.Error("fundamentals < 1h should be fresh regardless of market")
	}
	if r(2*time.Hour, true) || r(2*time.Hour, false) {
		t.Error("fundamentals > 1h should be stale regardless of market")
	}
}

func TestPathSanitization(t *testing.T) {
	s, _ := Open(t.TempDir(), true)
	// A hostile key with path traversal MUST stay inside the cache dir.
	if err := s.Put("../etc", "../../passwd", "nope"); err != nil {
		t.Fatal(err)
	}
	// File must be under s.Dir() regardless of the traversal attempt.
	var found string
	_ = filepath.Walk(s.Dir(), func(path string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			found = path
		}
		return nil
	})
	if found == "" {
		t.Fatal("no file written")
	}
	absDir, _ := filepath.Abs(s.Dir())
	absFound, _ := filepath.Abs(found)
	if !strings.HasPrefix(absFound, absDir+string(filepath.Separator)) {
		t.Fatalf("file escaped cache dir: %s (dir=%s)", absFound, absDir)
	}
}

func TestCorruptedEnvelopeReturnsError(t *testing.T) {
	s, _ := Open(t.TempDir(), true)
	path, _ := s.path(NSHoldings, "k")
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	_ = os.WriteFile(path, []byte("not json"), 0o600)
	_, hit, err := s.Get(NSHoldings, "k")
	if err == nil {
		t.Fatal("expected decode error")
	}
	// hit==false is explicitly part of the contract.
	if hit {
		t.Error("corrupted entry should not be a hit")
	}
}
