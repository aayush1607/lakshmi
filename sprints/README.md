# 🪷 Lakshmi — Sprint Plan

Three progressive sprints. Each feature is independently testable, and each sprint builds on the previous one. Technical design comes later; these specs describe **what** and **why**, not **how**.

---

## Sprint 1 — Foundation & The Grounded Answer
**Theme:** "A user can install Lakshmi, connect their broker, see their portfolio, and get one grounded answer."

Ship a working shell + a trustworthy answer loop over read-only data. No analytics yet.

| # | Feature | Outcome |
|---|---|---|
| [F1.1](sprint-1/F1.1-repl-shell.md) | REPL shell | A prompt the user can type into |
| [F1.2](sprint-1/F1.2-zerodha-login.md) | Zerodha login | Authenticated session stored locally |
| [F1.3](sprint-1/F1.3-portfolio-read.md) | Portfolio read-only view | `/portfolio` shows holdings |
| [F1.4](sprint-1/F1.4-local-store.md) | Local data store | Holdings/prices cached locally |
| [F1.5](sprint-1/F1.5-ask-grounded.md) | Grounded `ask` command | Free-form Q&A with citations |
| [F1.6](sprint-1/F1.6-response-shaper.md) | Universal verdict format | Every answer: Verdict → Why → Sources |

---

## Sprint 2 — Analysis Engines
**Theme:** "A user can analyse any stock (TA + FA) and their own portfolio in depth, all grounded."

Add the compute layer: local indicators, fundamentals pipeline, portfolio intelligence. The **planner loop (F2.0)** lands first because every downstream tool becomes dramatically more useful once the agent can choose and compose them on its own.

| # | Feature | Outcome |
|---|---|---|
| [F2.0](sprint-2/F2.0-agentic-planner.md) | Agentic planner loop | Agent picks its own tools per question |
| [F2.1](sprint-2/F2.1-nse-data-pipeline.md) | NSE OHLCV pipeline | Daily price history available offline |
| [F2.2](sprint-2/F2.2-ta-command.md) | `/ta <ticker>` | Technical verdict + signals |
| [F2.3](sprint-2/F2.3-chart-command.md) | `/chart <ticker>` | In-terminal price chart |
| [F2.4](sprint-2/F2.4-fa-command.md) | `/fa <ticker>` | Fundamental verdict + peer context |
| [F2.5](sprint-2/F2.5-peers-command.md) | `/peers <ticker>` | Peer ranking on canonical metrics |
| [F2.6](sprint-2/F2.6-portfolio-health.md) | `/portfolio health` | Concentration, sector tilt, quality score |
| [F2.7](sprint-2/F2.7-portfolio-rebalance.md) | `/portfolio rebalance` | Actionable trim/add suggestions |
| [F2.8](sprint-2/F2.8-news-filings.md) | News, filings & symbol resolution tools | Agent can answer "why is X down?" |

---

## Sprint 3 — Automation & Long-Horizon
**Theme:** "A user can codify playbooks, run live dashboards, set alerts, and plan long-term portfolios."

Convert Lakshmi from a query tool into a **companion** that keeps working when the user isn't typing.

| # | Feature | Outcome |
|---|---|---|
| [F3.1](sprint-3/F3.1-mantra-engine.md) | Mantra engine + 3 built-ins | Runnable YAML playbooks |
| [F3.2](sprint-3/F3.2-strategy-builder.md) | Conversational strategy builder | User-built backtested mantra |
| [F3.3](sprint-3/F3.3-darshans.md) | Darshans (3 dashboards) | Live portfolio / equity / market views |
| [F3.4](sprint-3/F3.4-aartis-alerts.md) | Aartis (alerts) | YAML-defined notifications |
| [F3.5](sprint-3/F3.5-yojanas.md) | Yojanas (long-horizon portfolios) | Coffee-can, all-weather, goal-based |
| [F3.6](sprint-3/F3.6-portfolio-tax-risk.md) | Tax / risk / attribution | Deeper portfolio lenses |

---

## Where the moat lives
Sprint 1 is plumbing anyone can build (LLM + MCPs + TUI). The defensibility is in **Sprint 2** (clean, adjusted, point-in-time Indian-market data + deterministic compute) and the **workflow store** in Sprint 3 (mantras / yojanas / aartis that users accumulate over time). Features that contribute to the moat are tagged **🏰 Moat contribution** inside their spec. Treat those as non-negotiable on correctness, even if it means spending longer than planned.

## Data strategy (v1, at a glance)
- **No paid APIs, no partnerships.** v1 ships on free Tier-1 sources.
- **Prices** — NSE + BSE bhavcopy, corporate actions parsed in-house. Full details in [F2.1](sprint-2/F2.1-nse-data-pipeline.md).
- **Fundamentals** — Screener.in scraped politely (Tier 2) for v1, replaced by XBRL-parsed filings (Tier 1) in v1.5. Full details in [F2.4](sprint-2/F2.4-fa-command.md).
- **Filings, shareholding, insider trades** — SEBI / NSE / BSE public feeds directly.
- **News** — RSS headlines only (Moneycontrol / LiveMint / BusinessLine).
- **LLM** — BYO API key from users in beta; zero inference cost to us.
- **Cost to run v1: ₹0 recurring.** First paid dependency (Screener license or LLM bill) only when we launch a paid tier.

---

## How to read a feature file
Each file has:
1. **One-liner** — the pitch
2. **User story** — who wants this, why
3. **Expected behaviour (PM view)** — what the user sees and experiences
4. **Acceptance criteria** — how we know it's done
5. **Out of scope** — explicit non-goals
6. **Dependencies** — which earlier features it needs
