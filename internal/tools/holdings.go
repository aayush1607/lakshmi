package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aayush1607/lakshmi/internal/broker"
	"github.com/aayush1607/lakshmi/internal/cache"
	"github.com/aayush1607/lakshmi/internal/portfolio"
)

// HoldingsTool exposes the user's Zerodha holdings. It consults the
// local cache first (same freshness rules as /portfolio) and only then
// falls through to the live broker call. That keeps the agent cheap
// and fast on repeated questions within a session.
type HoldingsTool struct {
	Broker broker.Broker
	Cache  *cache.Store
	Now    func() time.Time
}

// NewHoldingsTool is a convenience constructor.
func NewHoldingsTool(b broker.Broker, c *cache.Store) *HoldingsTool {
	return &HoldingsTool{Broker: b, Cache: c, Now: time.Now}
}

// Name implements Tool.
func (t *HoldingsTool) Name() string { return "portfolio_holdings" }

// Description implements Tool.
func (t *HoldingsTool) Description() string {
	return "Returns the user's current Zerodha equity holdings (symbol, quantity, average cost, LTP, close)."
}

// Call implements Tool. No args are read yet — the whole portfolio is
// returned and the agent slices it as needed.
func (t *HoldingsTool) Call(ctx context.Context, _ map[string]any) (Result, error) {
	now := t.Now()
	marketOpen := portfolio.MarketOpen(now)

	// Load session first: required for cache keying and also so we can
	// fail fast with a meaningful error if the user is not logged in.
	sess, err := t.Broker.Session()
	if err != nil {
		return Result{}, err
	}
	key := sess.UserID
	if key == "" {
		key = "default"
	}

	// Cache path. When the cache is disabled Get is a clean miss.
	if t.Cache != nil {
		if entry, hit, _ := t.Cache.Get(cache.NSHoldings, key); hit {
			if rule := cache.Rules[cache.NSHoldings]; rule != nil && rule(entry.Age, marketOpen) {
				var h []broker.Holding
				if err := json.Unmarshal(entry.Payload, &h); err == nil {
					return holdingsResult(h, entry.FetchedAt, true), nil
				}
			}
		}
	}

	// Live fetch.
	h, err := t.Broker.Holdings(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("holdings: %w", err)
	}
	if t.Cache != nil {
		_ = t.Cache.Put(cache.NSHoldings, key, h)
	}
	return holdingsResult(h, now.UTC(), false), nil
}

func holdingsResult(h []broker.Holding, fetchedAt time.Time, fromCache bool) Result {
	_ = fromCache // reserved for future "stale" signalling in Result
	totals := portfolio.ComputeTotals(h)
	return Result{
		Data: map[string]any{
			"as_of":    fetchedAt.Format(time.RFC3339),
			"holdings": compactHoldings(h),
			"totals": map[string]any{
				"invested":        round2(totals.Invested),
				"current":         round2(totals.Current),
				"overall_pnl":     round2(totals.OverallPnL),
				"overall_pnl_pct": round2(totals.OverallPct),
			},
		},
		Summary: fmt.Sprintf("%d holdings, invested ₹%s, current ₹%s",
			len(h), fmtMoney(totals.Invested), fmtMoney(totals.Current)),
		Sources: []Source{{
			Name:      "Zerodha Kite",
			URL:       "kite.zerodha.com",
			Tier:      2,
			FetchedAt: fetchedAt,
		}},
	}
}

type compactHolding struct {
	Symbol       string  `json:"symbol"`
	Qty          int     `json:"qty"`
	AvgCost      float64 `json:"avg_cost"`
	LTP          float64 `json:"ltp"`
	Value        float64 `json:"value"`
	PnL          float64 `json:"pnl"`
	WeightPct    float64 `json:"weight_pct"`
}

// compactHoldings trims Kite's vendor fields so the Reason prompt stays
// small and predictable. Values are rounded to 2dp to avoid the LLM
// echoing spurious precision.
func compactHoldings(h []broker.Holding) []compactHolding {
	totalValue := 0.0
	for _, x := range h {
		totalValue += float64(x.Quantity) * x.LTP
	}
	out := make([]compactHolding, 0, len(h))
	for _, x := range h {
		value := float64(x.Quantity) * x.LTP
		pnl := float64(x.Quantity) * (x.LTP - x.AvgCost)
		weight := 0.0
		if totalValue > 0 {
			weight = 100 * value / totalValue
		}
		out = append(out, compactHolding{
			Symbol:    x.Symbol,
			Qty:       x.Quantity,
			AvgCost:   round2(x.AvgCost),
			LTP:       round2(x.LTP),
			Value:     round2(value),
			PnL:       round2(pnl),
			WeightPct: round2(weight),
		})
	}
	return out
}

func round2(v float64) float64 {
	// Cheap, deterministic 2dp round.
	return float64(int64(v*100+sign(v)*0.5)) / 100
}

func sign(v float64) float64 {
	if v < 0 {
		return -1
	}
	return 1
}

func fmtMoney(v float64) string {
	// Simple rupee formatting for summary lines only. No grouping.
	return fmt.Sprintf("%.0f", v)
}
