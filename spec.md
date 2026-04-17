# 🪷 Lakshmi.sh — Feature Rethink v2

Great refinement. You've moved the product from *"another terminal with AI"* → *"a **trustworthy, grounded, fast** research companion."* That's the right evolution. Let me restructure features around your constraints.

---

## 🎯 Guiding Principles (The New Constitution)

| # | Principle | What it means in practice |
|---|---|---|
| 1 | **Local-first, cloud-minimal** | Only LLM API calls + MCP calls leave the machine. Everything else runs locally. |
| 2 | **Grounded or silent** | Every factual claim cites a source. No citation = no claim. Agent refuses rather than hallucinates. |
| 3 | **Trusted sources only** | Curated allowlist (NSE, BSE, SEBI, RBI, company filings, Moneycontrol, Reuters). No Reddit, no Telegram, no random blogs. |
| 4 | **Direct answer first** | Yes/No/Maybe → then the "why" → then the detail. Never bury the lede. |
| 5 | **Simple > Clever** | A feature that's 80% right and shippable beats 100% right and fragile. |
| 6 | **Readable code** | ~300 LOC per file max. Pure functions where possible. One concept per package. |

---

## 🏗️ Simplified Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    lakshmi (single Go binary)            │
│                                                          │
│   ┌─────────┐    ┌──────────┐    ┌─────────────────┐    │
│   │  REPL   │───▶│  Agent   │───▶│ Response Shaper │    │
│   │  (TUI)  │    │  Loop    │    │ (verdict+why)   │    │
│   └─────────┘    └────┬─────┘    └─────────────────┘    │
│                       │                                  │
│                ┌──────┴──────┐                          │
│                ▼             ▼                          │
│         ┌───────────┐  ┌──────────┐                     │
│         │ Tool Hub  │  │  Cache   │                     │
│         │ (MCP+HTTP)│  │ (DuckDB) │                     │
│         └─────┬─────┘  └──────────┘                     │
└───────────────┼──────��─────────────────────────────────┘
                │
     ┌──────────┼──────────┬──────────┬──────────┐
     ▼          ▼          ▼          ▼          ▼
  Zerodha    NSE/BSE    Screener  Moneycontrol  LLM
   MCP       APIs        .in       (RSS)        API
```

**That's the entire system.** 5 components. No microservices. No queues. No Kubernetes. Single binary. ~5000 LOC target for v1.

---

## 🧱 Feature Blocks (Rescoped)

### 🎙️ Block 1 — **The Grounded Answer Engine** *(the heart)*

Every query goes through a standard **5-phase loop**:

```
 1. UNDERSTAND  → classify intent (portfolio | stock | strategy | explain)
 2. PLAN        → pick tools needed (show plan to user)
 3. FETCH       → parallel tool calls, with source URLs captured
 4. REASON      → LLM synthesizes ONLY from fetched context (RAG-pure)
 5. SHAPE       → verdict → reason → detail → sources → next actions
```

**The universal response format:**

```
🪷 › should I trim my TCS position?

  ━━━ VERDICT ━━━
  🟡 MAYBE — lean toward trimming 25-30%

  ━━━ WHY ━━━
  • Valuation: FY26 P/E 28x vs 5yr avg 24x (stretched)
  • Concentration: 14% of your portfolio (above your 10% rule)
  • Momentum: +22% in 45 days, RSI 71 (overbought)

  ━━━ DETAIL ━━━
  [expanded analysis with tables, peers, scenarios]

  ━━━ SOURCES ━━━
  [1] screener.in/company/TCS — fundamentals
  [2] nseindia.com — price data
  [3] your zerodha holdings (local)

  ━━━ CONFIDENCE: 78% ━━━
  Based on 3 sources · 0 conflicts · recency: today

  📎 Next: /peers TCS  /scenario trim-30  /tax-impact
```

**Why this works:** Users get the decision in 1 second, the context in 5, and the depth on demand. No scrolling through LLM waffle.

---

### 💼 Block 2 — **Portfolio Intelligence** *(the daily driver)*

**Capabilities:**

| Command | What it does | Grounded on |
|---|---|---|
| `/portfolio health` | 10-second portfolio checkup: concentration, sector tilt, quality score | Zerodha holdings + Screener fundamentals |
| `/portfolio rebalance` | Suggests specific trim/add actions with rationale | Holdings + target allocation + current valuations |
| `/portfolio attribute` | "Why up/down today?" with per-stock contribution | Holdings + intraday prices + news |
| `/portfolio risk` | Concentration, beta, drawdown, sector clustering | Historical prices + holdings |
| `/portfolio tax` | Harvesting opportunities, LTCG/STCG pockets | Holdings + cost basis + tax rules |

**The rebalancing flow:**

```
🪷 › /portfolio rebalance

  ━━━ CURRENT ALLOCATION ━━━
  IT          ███████████████████░ 38%  (target: 25%)  ⚠️
  Banks       ████████░░░░░░░░░░░░ 18%  (target: 25%)
  Auto        ██████░░░░░░░░░░░░░░ 14%  (target: 15%)  ✓
  Pharma      ████░░░░░░░░░░░░░░░░ 10%  (target: 15%)
  Energy      ████░░░░░░░░░░░░░░░░ 10%  (target: 10%)  ✓
  Cash        ██░░░░░░░░░░░░░░░░░░  5%

  ━━━ SUGGESTED ACTIONS ━━━
  
  1. TRIM  TCS         ₹45,000   (P/E 28x vs 24x avg — rich)
  2. TRIM  INFY        ₹30,000   (recent run-up, concentration)
  3. ADD   HDFCBANK    ₹40,000   (FY26 P/B 2.1x, 5yr low, NIM stable)
  4. ADD   SUN PHARMA  ₹35,000   (US gx approval pipeline, clean BS)

  Expected post-rebalance:
  IT: 28%  Banks: 23%  Pharma: 14% (closer to target)

  ⚠️  Tax impact: ~₹8,200 STCG if executed today.
      Wait 11 days for LTCG on TCS → saves ₹5,100.

  ━━━ SOURCES ━━━
  [1] screener.in — all fundamentals
  [2] zerodha holdings + cost basis
  [3] your stated targets (~/.lakshmi/targets.yaml)

  📎 Execute in Kite: /export rebalance --to kite-basket
```

**Why this wins:** It's the *killer feature* for retail. Nobody does this well today. Screener shows fundamentals, Kite shows holdings, but nothing **synthesizes** them into *actionable, tax-aware, grounded* suggestions.

---

### 📊 Block 3 — **Technical Analysis On-Demand**

**Principle:** Don't build a charting platform. Build a *charting answer engine*.

| Command | What it does |
|---|---|
| `/ta <ticker>` | Full technical snapshot: trend, momentum, support/resistance, volume |
| `/chart <ticker> [timeframe]` | Beautiful in-terminal candlestick chart with overlays |
| `/pattern <ticker>` | Detects classical patterns (head-shoulders, triangles, flags) with confidence |
| `/levels <ticker>` | Key S/R levels with touch-count and strength |
| `/compare TCS INFY WIPRO` | Side-by-side normalized price chart |

**Sample output:**

```
🪷 › /ta HDFCBANK

  HDFCBANK · ₹1,642.30 (+0.8%) · Vol 2.1x avg

  ┌─ 3M PRICE ────────────────────────────────────┐
  │                                    ╭──╮       │
  │                                 ╭──╯  ╰╮      │
  │                      ╭──╮    ╭──╯      ╰──    │
  │           ╭──╮    ╭──╯  ╰────╯                │
  │    ╭─────╯  ╰────╯                            │
  │ ───╯                                          │
  │  1520        1580        1620         1642   │
  └───────────────────────────────────────────────┘

  ━━━ VERDICT ━━━
  🟢 BULLISH — confirmed uptrend, momentum intact

  ━━━ SIGNALS ━━━
  Trend        ▲ Uptrend (20/50/200 DMA aligned)
  Momentum     ▲ RSI 62 (strong, not overbought)
  Volume       ▲ 2.1x avg (accumulation)
  Pattern      ▲ Ascending triangle, ~85% confidence

  ━━━ KEY LEVELS ━━━
  Resistance   ₹1,680  (3 touches · strong)
  Support      ₹1,595  (2 touches · recent)
  Stop-loss    ₹1,578  (below swing low)

  ━━━ SOURCES ━━━
  [1] NSE bhavcopy (6 months daily OHLCV)
  [2] computed locally (no external TA service)
```

**Key choice:** TA is **computed locally** in Go (no external dependency). We pull OHLCV from NSE, compute indicators in `internal/ta/`. This is fast, free, and verifiable.

---

### 🔬 Block 4 — **Fundamental Analysis**

| Command | What it does |
|---|---|
| `/fa <ticker>` | One-screen fundamental summary with peer comparison |
| `/dcf <ticker>` | Interactive DCF — agent proposes assumptions, you tweak |
| `/peers <ticker>` | Peer ranking on 8 canonical metrics |
| `/quality <ticker>` | Quality score: balance sheet, cash flow, governance |
| `/concall <ticker>` | RAG over last 4 concall transcripts — ask anything |

**The `/fa` verdict format:**

```
🪷 › /fa TCS

  ━━━ VERDICT ━━━
  🟢 HIGH-QUALITY  |  🟡 FULLY-VALUED  |  🟢 GROWTH-STABLE

  ━━━ QUALITY (9/10) ━━━
  ROE 52%   ROCE 63%   D/E 0.08   Cash ₹38,000 Cr

  ━━━ VALUATION (5/10) ━━━
  P/E  28x (5yr avg 24x, sector 26x)  ← stretched
  P/B  12x (5yr avg 10x)
  EV/EBITDA 21x

  ━━━ GROWTH (7/10) ━━━
  Rev CAGR 3y: 12%   NPM trend: stable 19-20%
  Deal TCV Q3: $13B (guidance beat)

  ━━━ SOURCES ━━━
  [1] screener.in · [2] TCS Q3 FY26 results · [3] concall transcript
```

---

### 🔁 Block 5 — **Strategy & Playbook Builder**

**The flow is conversational, not form-filling:**

```
🪷 › help me build a momentum strategy for mid-caps

  ━━━ I'll help you build this. First, let me clarify: ━━━
  
  1. Universe?  [default: NIFTY Midcap 150]        → accept / change
  2. Momentum?  [default: 12-1 month returns]      → accept / change
  3. Rebalance? [default: monthly]                 → accept / change
  4. Positions? [default: top 15 equal-weight]     → accept / change
  5. Filters?   [default: min ₹500Cr avg volume]   → accept / change

  🪷 › accept

  ⟳ generating strategy…
  ⟳ backtesting 2019-2025 on NSE data…

  ━━━ STRATEGY: "MidMomo-15" ━━━
  
  Backtest summary (2019-01 to 2025-12):
  CAGR           18.4%  vs NIFTY Midcap 14.2%
  Max DD         -32%   vs -38%
  Sharpe         1.12
  Hit rate       54%
  Turnover       ~85% annual

  Monte Carlo (1000 runs, bootstrap):
  P5 CAGR 9.8%  |  P50 17.2%  |  P95 24.1%

  ━━━ SAVED AS ━━━
  ~/.lakshmi/mantras/midmomo-15.yaml

  📎 Next:
     /mantra run midmomo-15 --paper   (paper trade it)
     /mantra edit midmomo-15           (tweak params)
     /mantra share midmomo-15          (export to marketplace)
```

**What this is:**
- A **Mantra** (YAML playbook) that's version-controlled
- Backtested on real local data (NSE bhavcopy archive, ~2GB)
- Runnable in paper mode or (later) live mode
- Shareable as plain YAML

**What this isn't:** A QuantConnect clone. We don't build a full backtesting engine. We build a **constrained, opinionated** one that handles ~90% of retail strategies (long-only, daily/weekly rebalance, simple factor combos).

---

### 📅 Block 6 — **Portfolio Investing Strategies**

Longer-horizon cousin of Block 5, for wealth-building rather than trading:

| Command | What it does |
|---|---|
| `/yojana coffee-can` | Build a coffee-can portfolio (high ROCE, low debt, 10y+ track record) |
| `/yojana all-weather` | Ray-Dalio-style balanced allocation adapted for Indian markets |
| `/yojana goal --amount 2Cr --years 10` | Reverse-engineer portfolio from a financial goal |
| `/yojana smallcase <name>` | Import a smallcase, analyze it, suggest improvements |
| `/yojana stress --scenario "2008 repeat"` | Scenario test current portfolio |

**Each Yojana outputs:**
1. The **allocation** (what to buy, in what %)
2. The **rationale** (why each pick, with sources)
3. The **rebalance rule** (when to adjust)
4. The **expected behavior** (CAGR range, max DD, recovery time — from backtest)

---

### 🧭 Block 7 — **The Darshans (Live Dashboards)**

Simplified to **3 flagship dashboards** in v1:

| Darshan | Purpose |
|---|---|
| `darshan portfolio` | Your live P&L, allocation, day attribution |
| `darshan equity <ticker>` | One-screen summary of everything knowable |
| `darshan market` | NIFTY/BANKNIFTY, advance-decline, FII/DII, India VIX, top movers |

**Design rule:** Each darshan is one screen. No scrolling. No tabs. If it doesn't fit, it's not essential.

---

### 🔔 Block 8 — **Aartis (Alerts)**

Simple, deterministic, local-first:

```yaml
# ~/.lakshmi/aartis/tcs-trim.yaml
name: "TCS trim signal"
when:
  - symbol: TCS
    pe: "> 28"
    rsi_14: "> 70"
notify:
  - terminal
  - telegram
cooldown: 24h
```

No complex event engine. Just a 1-minute cron that evaluates YAML conditions and fires notifications.

---

## 🔒 Source Trust Tiers

```
TIER 1 — Primary (always prefer)
  • NSE / BSE bhavcopy and corporate filings
  • SEBI, RBI, MCA filings
  • Company investor relations pages
  • Audited annual reports

TIER 2 — Licensed aggregators
  • Screener.in (via API/licensed)
  • Trendlyne, Tijori
  • Zerodha / broker APIs

TIER 3 — Curated news
  • Moneycontrol, Reuters, Bloomberg headlines
  • LiveMint, Economic Times
  • Company press releases

❌ NEVER
  • Twitter/X, Reddit, Telegram, WhatsApp
  • Unverified blogs, YouTube
  • "Tips" services
```

**Every response shows the tier of its sources.** If an answer relies only on Tier 3, it's flagged. If any Tier 1 source is available, it's prioritized.

---

## 🚫 Explicit De-scoping (v1)

To keep it simple, we **cut** from the original spec:

| Cut | Reason |
|---|---|
| Multi-agent DAG orchestrator | Over-engineered for v1. Single agent + tools is enough. |
| Mantra marketplace | v2. Need users first. |
| Custom MCP SDK | v2. Use native MCPs only for now. |
| Voice input | Gimmick. |
| Hindi Mantras | v1.5. |
| PDF memo export | v1.5. |
| Multi-broker abstraction | v1.5. Zerodha first. |
| Live trading | Post-audit only. v2. |
| Options IV surface | Complex, narrow audience. v1.5. |

---

## 🧩 Revised Tech Map

| Concern | Choice | Why |
|---|---|---|
| Binary | Go 1.22 | Single binary, fast, easy distribution |
| TUI | Bubbletea + Lipgloss | Best-in-class TUI ecosystem |
| CLI | Cobra | Standard |
| Local store | DuckDB + Parquet | SQL on your laptop, 10x smaller than Postgres |
| Cache | BadgerDB (KV) | Embedded, no server |
| MCP client | `mcp-go` | Reference impl |
| LLM | Anthropic primary, OpenAI fallback | Best reasoning; swap via config |
| Config | YAML (~/.lakshmi/) | Human-readable, versionable |
| TA indicators | Pure Go (in-tree) | Zero dependency, auditable |
| Charts | Unicode blocks + Kitty protocol | No external renderer |

**Total external dependencies target: < 20 Go modules.**

---

## 📏 Success Criteria for v1

A v1 is "done" when a user can, in < 10 minutes from install:

1. ✅ Link Zerodha (`lakshmi login`)
2. ✅ See their portfolio health (`/portfolio health`) with grounded verdict
3. ✅ Get a rebalancing suggestion (`/portfolio rebalance`) with sources
4. ✅ Run full TA on any ticker (`/ta TCS`) with chart
5. ✅ Run full FA on any ticker (`/fa TCS`) with peer comparison
6. ✅ Ask free-form questions and get **verdict → why → detail → sources**
7. ✅ Build and backtest a simple strategy conversationally
8. ✅ Set up one alert (Aarti)

Everything else is bonus.

---

## 📋 Revised 12-Week Roadmap

| Week | Focus | Ship |
|---|---|---|
| 1 | Shell + Cobra + Bubbletea REPL | Banner + prompt + history |
| 2 | Zerodha MCP + local store | `/portfolio` read-only |
| 3 | LLM agent loop + grounding | `ask` with sources |
| 4 | Response shaper (verdict format) | All answers look consistent |
| 5 | NSE data pipeline + TA engine | `/ta`, `/chart`, `/levels` |
| 6 | Fundamentals pipeline | `/fa`, `/peers`, `/quality` |
| 7 | Portfolio analytics | `/portfolio health`, `/rebalance` |
| 8 | Mantra engine + 3 built-ins | `mantra run morning-brief` |
| 9 | Darshans (3 dashboards) | `darshan portfolio/equity/market` |
| 10 | Strategy builder + mini backtest | Conversational strategy creation |
| 11 | Aartis (alerts) + polish | E2E feature-complete |
| 12 | Docs, demo video, launch | **Public beta** |

---

## 🎬 The Pitch, Refined

> **Lakshmi is the grounded thinking partner for the Indian investor.**
>
> Every answer starts with a verdict. Every claim cites a source. Every source is trusted.
>
> It knows your portfolio (Zerodha), your market (NSE/BSE), your stocks (Screener, filings), and your targets (yours). It computes locally, calls LLMs sparingly, and never pretends.
>
> It runs in your terminal. It's a single binary. It's open source.
>
> *Think of it as Bloomberg's brain, with Warren Buffett's discipline, at Zerodha's price.*

---
