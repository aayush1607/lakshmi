package tools

import (
	"context"
	"strings"
	"time"
)

// SectorLookupTool is a trivial hardcoded NSE-symbol → sector map.
// The tech plan explicitly flags "Sector/industry mapping beyond a
// trivial lookup" as out of scope for Sprint 1, so this table covers
// the commonly-held Nifty-50 / Next-50 universe and nothing else.
// Unknown symbols are reported as "Unknown" rather than silently
// dropped — the Reason step needs to know when it lacks grounding.
//
// Tier 3 source ("Lakshmi static sector map"): derived, not official.
// Upgrade path: Sprint 2 replaces this with an NSE CM Sectoral Indices
// lookup + live membership of the sectoral index constituents.
type SectorLookupTool struct {
	Now func() time.Time
}

// NewSectorLookupTool returns a new instance with time.Now bound.
func NewSectorLookupTool() *SectorLookupTool {
	return &SectorLookupTool{Now: time.Now}
}

func (t *SectorLookupTool) Name() string { return "sector_lookup" }

func (t *SectorLookupTool) Description() string {
	return "Maps a list of NSE trading symbols to their sector. Sector vocabulary is fixed and Indian-market specific."
}

// Call accepts {"symbols":["TCS","HDFCBANK",…]} and returns the mapping.
// When symbols is empty, returns the full static map (useful for
// broad "what's my X exposure?" questions).
func (t *SectorLookupTool) Call(_ context.Context, args map[string]any) (Result, error) {
	var symbols []string
	if raw, ok := args["symbols"]; ok {
		switch v := raw.(type) {
		case []string:
			symbols = v
		case []any:
			for _, s := range v {
				if ss, ok := s.(string); ok {
					symbols = append(symbols, ss)
				}
			}
		}
	}
	out := map[string]string{}
	unknown := []string{}
	if len(symbols) == 0 {
		for k, v := range staticSectorMap {
			out[k] = v
		}
	} else {
		for _, s := range symbols {
			key := strings.ToUpper(strings.TrimSpace(s))
			if sector, ok := staticSectorMap[key]; ok {
				out[key] = sector
			} else {
				out[key] = "Unknown"
				unknown = append(unknown, key)
			}
		}
	}
	summary := "mapped "
	if len(symbols) == 0 {
		summary += "full sector dictionary"
	} else {
		summary += joinInt(len(symbols)) + " symbols"
	}
	if len(unknown) > 0 {
		summary += " (" + joinInt(len(unknown)) + " unknown)"
	}
	return Result{
		Data: map[string]any{
			"sectors": out,
			"unknown": unknown,
		},
		Summary: summary,
		Sources: []Source{{
			Name:      "Lakshmi static sector map",
			Tier:      3,
			FetchedAt: t.Now().UTC(),
		}},
	}, nil
}

func joinInt(n int) string {
	switch n {
	case 0:
		return "0"
	}
	const digits = "0123456789"
	if n < 10 {
		return string(digits[n])
	}
	// small positive ints only; good enough for summaries.
	buf := make([]byte, 0, 4)
	for n > 0 {
		buf = append([]byte{digits[n%10]}, buf...)
		n /= 10
	}
	return string(buf)
}

// staticSectorMap is the Sprint 1 grounding for sector exposure. Kept
// small on purpose — only tickers a typical Indian retail portfolio
// actually holds. "Unknown" is a first-class outcome; we do not guess.
var staticSectorMap = map[string]string{
	// IT Services
	"TCS":        "IT Services",
	"INFY":       "IT Services",
	"WIPRO":      "IT Services",
	"HCLTECH":    "IT Services",
	"TECHM":      "IT Services",
	"LTIM":       "IT Services",
	"PERSISTENT": "IT Services",
	"COFORGE":    "IT Services",
	"MPHASIS":    "IT Services",

	// Banks
	"HDFCBANK":   "Banks",
	"ICICIBANK":  "Banks",
	"SBIN":       "Banks",
	"AXISBANK":   "Banks",
	"KOTAKBANK":  "Banks",
	"INDUSINDBK": "Banks",
	"BANKBARODA": "Banks",
	"PNB":        "Banks",
	"AUBANK":     "Banks",
	"IDFCFIRSTB": "Banks",

	// Non-bank financials
	"BAJFINANCE":  "NBFC",
	"BAJAJFINSV":  "NBFC",
	"CHOLAFIN":    "NBFC",
	"HDFCLIFE":    "Insurance",
	"SBILIFE":     "Insurance",
	"ICICIGI":     "Insurance",
	"ICICIPRULI":  "Insurance",
	"LICI":        "Insurance",

	// Energy / Oil & Gas
	"RELIANCE": "Energy",
	"ONGC":     "Energy",
	"IOC":      "Energy",
	"BPCL":     "Energy",
	"HINDPETRO": "Energy",
	"GAIL":     "Energy",
	"ADANIGREEN": "Energy",
	"NTPC":     "Power",
	"POWERGRID": "Power",
	"TATAPOWER": "Power",

	// Consumer / FMCG
	"HINDUNILVR": "FMCG",
	"ITC":        "FMCG",
	"NESTLEIND":  "FMCG",
	"BRITANNIA":  "FMCG",
	"DABUR":      "FMCG",
	"MARICO":     "FMCG",
	"TATACONSUM": "FMCG",
	"COLPAL":     "FMCG",
	"GODREJCP":   "FMCG",

	// Auto
	"MARUTI":     "Auto",
	"M&M":        "Auto",
	"TATAMOTORS": "Auto",
	"BAJAJ-AUTO": "Auto",
	"EICHERMOT":  "Auto",
	"HEROMOTOCO": "Auto",
	"TVSMOTOR":   "Auto",

	// Pharma / Healthcare
	"SUNPHARMA":   "Pharma",
	"DRREDDY":     "Pharma",
	"CIPLA":       "Pharma",
	"DIVISLAB":    "Pharma",
	"APOLLOHOSP":  "Healthcare",
	"LUPIN":       "Pharma",
	"TORNTPHARM":  "Pharma",

	// Materials
	"ULTRACEMCO": "Cement",
	"GRASIM":     "Cement",
	"SHREECEM":   "Cement",
	"AMBUJACEM":  "Cement",
	"TATASTEEL":  "Metals",
	"JSWSTEEL":   "Metals",
	"HINDALCO":   "Metals",
	"VEDL":       "Metals",
	"COALINDIA":  "Metals",

	// Telecom & Services
	"BHARTIARTL":   "Telecom",
	"IDEA":         "Telecom",
	"ASIANPAINT":   "Paints",
	"PIDILITIND":   "Chemicals",
	"DMART":        "Retail",
	"TRENT":        "Retail",
	"ZOMATO":       "Consumer Tech",
	"NYKAA":        "Consumer Tech",
	"PAYTM":        "Consumer Tech",
	"POLICYBZR":    "Consumer Tech",
	"INDIGO":       "Aviation",
	"LT":           "Capital Goods",
	"SIEMENS":      "Capital Goods",
	"ABB":          "Capital Goods",
	"BEL":          "Defence",
	"HAL":          "Defence",
	"TITAN":        "Consumer Durables",
	"HAVELLS":      "Consumer Durables",
	"ADANIENT":     "Conglomerate",
	"ADANIPORTS":   "Infrastructure",
	"DLF":          "Real Estate",
}
