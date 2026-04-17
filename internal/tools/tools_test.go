package tools

import (
	"context"
	"testing"
	"time"
)

func TestSectorLookupKnown(t *testing.T) {
	st := NewSectorLookupTool()
	r, err := st.Call(context.Background(), map[string]any{
		"symbols": []any{"TCS", "HDFCBANK", "BOGUS"},
	})
	if err != nil {
		t.Fatal(err)
	}
	data := r.Data.(map[string]any)
	sectors := data["sectors"].(map[string]string)
	if sectors["TCS"] != "IT Services" {
		t.Errorf("TCS = %q", sectors["TCS"])
	}
	if sectors["HDFCBANK"] != "Banks" {
		t.Errorf("HDFCBANK = %q", sectors["HDFCBANK"])
	}
	if sectors["BOGUS"] != "Unknown" {
		t.Errorf("BOGUS = %q", sectors["BOGUS"])
	}
	if len(r.Sources) != 1 || r.Sources[0].Tier != 3 {
		t.Errorf("sources = %+v", r.Sources)
	}
}

func TestSectorLookupFullMapWhenEmpty(t *testing.T) {
	st := NewSectorLookupTool()
	r, _ := st.Call(context.Background(), map[string]any{})
	sectors := r.Data.(map[string]any)["sectors"].(map[string]string)
	if _, ok := sectors["TCS"]; !ok {
		t.Error("expected full dictionary to include TCS")
	}
	if len(sectors) < 40 {
		t.Errorf("static map seems too small: %d entries", len(sectors))
	}
}

func TestTimeNow(t *testing.T) {
	ist := time.FixedZone("IST", 5*3600+30*60)
	tool := &TimeNowTool{Now: func() time.Time {
		return time.Date(2026, 4, 17, 10, 0, 0, 0, ist) // Friday 10:00 IST
	}}
	r, err := tool.Call(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	data := r.Data.(map[string]any)
	if data["market_open"] != true {
		t.Errorf("expected market_open true on Fri 10:00 IST")
	}
	if len(r.Sources) != 0 {
		t.Errorf("time_now must not claim sources")
	}
}

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	r.Register(NewSectorLookupTool())
	r.Register(NewTimeNowTool())
	names := r.Names()
	if len(names) != 2 || names[0] != "sector_lookup" || names[1] != "time_now" {
		t.Fatalf("names = %v", names)
	}
	if _, ok := r.Get("sector_lookup"); !ok {
		t.Error("missing sector_lookup")
	}
	if _, ok := r.Get("missing"); ok {
		t.Error("unexpected hit for missing tool")
	}
}
