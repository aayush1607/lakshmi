# 🪷 lakshmi.sh

> wealth, intelligently.

A conversational, grounded stock-analysis terminal for Indian markets. Single Go binary, local-first, every claim cited.

Lakshmi is a **thinking partner for the Indian investor**. You type a question, it plans a small set of tool calls against your broker and public market data, and it returns a one-line verdict → a few supporting bullets → the raw detail on request → the sources behind every number. No claim reaches the screen without a citation. When the data can't support an answer, it refuses — on purpose.

The product is opinionated about three things:

- **Grounding is structural, not advisory.** Every factual claim carries an inline `[n]` citation; ungrounded answers are rejected by the agent before they reach you.
- **Local-first.** Your Kite token lives in the OS keychain. Your holdings and prices live under `~/.lakshmi/`. Nothing is uploaded anywhere.
- **One visual grammar.** Every answer — portfolio totals, free-form Q&A, errors, unknown commands — renders as Verdict → Why → Detail → Sources → Confidence → Next. The layout is the product.

**Status:** ✅ Sprint 1 shipped. Planning Sprint 2 (analysis engines + agentic planner). See [`spec.md`](spec.md) for the product spec and [`sprints/README.md`](sprints/README.md) for the build plan.

## What ships today

| Feature | Status | What it does |
|---|---|---|
| F1.1 REPL shell | ✅ | `₹ ›` prompt with history (↑/↓), `/help`, `/exit`, `:q`, inline-mode transcript that plays nicely with native scroll and copy‐paste |
| F1.2 Zerodha login | ✅ | Browser OAuth via Kite Connect, token in OS keychain, loopback callback on 127.0.0.1 |
| F1.3 Portfolio view | ✅ | `/portfolio` — holdings table with sort (`--by weight\|pnl\|symbol`), totals, live vs post-close note |
| F1.4 Local cache | ✅ | `~/.lakshmi/data/` JSON cache with per-namespace freshness rules, `--fresh` / `--no-cache`, `/cache` controls |
| F1.5 Grounded ask | ✅ | `/ask`, free-form questions, `/why` trace, one-retry-then-refuse on bad citations |
| F1.6 Universal verdict format | ✅ | Every answer: 🟢/🟡/🔴 verdict → why bullets → detail (collapses past 20 lines, `more` to expand) → sources → deterministic confidence → numbered next actions (type `1`/`2` to run) |

**Sprint 1 complete.** Next up is **Sprint 2** — an agentic planner loop that picks its own tools per question ([F2.0](sprints/sprint-2/F2.0-agentic-planner.md)), the NSE OHLCV pipeline, and `/ta` / `/fa` / `/peers` commands. See [`sprints/README.md`](sprints/README.md).

## Quick start (dev)

```bash
make build      # produces bin/lakshmi
./bin/lakshmi   # launches the REPL — type /help
make test       # run all tests
```

Requires Go 1.22+.

## Connecting to Zerodha

Lakshmi talks to your Zerodha account through **Kite Connect** (the official API), not through any third-party scraper or MCP wrapper. You need your own Kite Connect app — it takes five minutes.

### 1. Create a Kite Connect app

1. Go to [developers.kite.trade](https://developers.kite.trade) and sign in with your Zerodha credentials.
2. Click **Create new app** → type **Connect**.
3. Set the **Redirect URL** to exactly:
   ```
   http://127.0.0.1:7878/lakshmi/callback
   ```
4. Copy the **API key** and **API secret** shown after creation.

> Kite Connect charges ₹2,000/month per app for live data. Lakshmi has no commercial relationship with Zerodha — you own the app and the subscription.

### 2. Export your credentials

Put them in a dotfile that your shell sources on startup — **never** check them into git.

```bash
cat > ~/.lakshmi.env <<'EOF'
export KITE_API_KEY=your_api_key_here
export KITE_API_SECRET=your_api_secret_here
# optional — defaults to 7878; must match the redirect URL you registered
# export KITE_REDIRECT_PORT=7878
EOF

chmod 600 ~/.lakshmi.env
echo '[ -f ~/.lakshmi.env ] && source ~/.lakshmi.env' >> ~/.zshrc
source ~/.lakshmi.env
```

### 3. Log in

```bash
./bin/lakshmi login
```

What happens:

1. Lakshmi spins a short-lived HTTP listener on `127.0.0.1:7878`.
2. Your browser opens at Kite's login page.
3. You enter your Zerodha user ID / password / TOTP.
4. Kite redirects back to the local listener, which exchanges the request token for an access token.
5. The access token is written to your **OS keychain** (Keychain on macOS, Secret Service on Linux, Credential Manager on Windows).
6. Non-secret session metadata (user id, expiry) is written to `~/.lakshmi/session.json`.

Kite tokens expire at **06:00 IST every day**, so you will log in once per trading morning.

### 4. Check status / log out

```bash
./bin/lakshmi session    # shows who you're logged in as and when the session expires
./bin/lakshmi logout     # clears both the keychain token and the session file
```

The same commands exist inside the REPL as `/login`, `/login status`, and `/logout`. The REPL flow is async: the prompt stays responsive while the browser is open, and the success line appears when the flow completes.

## Viewing your portfolio

Once logged in, you can see your Zerodha holdings:

```bash
./bin/lakshmi portfolio                 # sorted by weight (default)
./bin/lakshmi portfolio --by pnl        # sorted by absolute P&L
./bin/lakshmi portfolio --by symbol     # alphabetical
# short aliases also work:
./bin/lakshmi p
./bin/lakshmi holdings
```

Inside the REPL:

```
₹ › /portfolio
₹ › /p --by pnl
₹ › /holdings
```

The table shows symbol, quantity, average cost, LTP, P&L (₹ and %), and portfolio weight. Beneath it: total invested, current value, day P&L, and overall P&L. The header notes whether the quote is **live** (09:15–15:30 IST, Mon–Fri) or a **post-close** snapshot. Read-only — Lakshmi has no code path that can place or modify orders.

## Local cache

Holdings, quotes, and fundamentals are cached under `~/.lakshmi/data/` as plain JSON files (`jq`-friendly, one entry per file). Cache-first semantics:

| Data | In market hours (09:15–15:30 IST, Mon–Fri) | Off-hours |
|---|---|---|
| Holdings | always refetch | cache good for 24 h |
| Quotes | always refetch | always serve cache (last close) |
| Fundamentals | serve cache if < 1 h old | same |

Flags and controls:

```bash
./bin/lakshmi portfolio --fresh     # bypass cache for this call
./bin/lakshmi --no-cache portfolio  # no reads, no writes, this process only
LAKSHMI_NO_CACHE=1 lakshmi          # same, as an env switch

./bin/lakshmi cache status          # what's cached, disk use, freshness
./bin/lakshmi cache clear           # wipe everything under data/
./bin/lakshmi cache on              # re-enable at runtime (REPL also: /cache on)
./bin/lakshmi cache off             # disable at runtime
```

Inside the REPL: `/cache status`, `/cache clear`, `/cache on`, `/cache off`. Toggling `/cache off` during a session means the next `/portfolio` will not read from disk and not write back — useful when debugging whether stale data is the problem.

The cache is single-user, local, never uploaded anywhere. It never stores your Kite access token (that's in the OS keychain) or your API credentials (those stay in your shell env).

## Asking questions (F1.5)

Lakshmi answers free-form questions about your portfolio by running a small deterministic tool chain — read holdings, look up sectors, check the clock — and then asking an LLM to summarise, **with the rule that every factual claim must cite a tool result**. Uncited answers are rejected. Hallucinated sources trigger one stricter retry, then a refusal.

### 1. Configure Azure AI Foundry

```bash
cat >> ~/.lakshmi.env <<'EOF'
export AZURE_FOUNDRY_ENDPOINT=https://your-resource.openai.azure.com
export AZURE_FOUNDRY_DEPLOYMENT=gpt-4o-mini        # whatever you deployed
export AZURE_FOUNDRY_API_KEY=your_key_here
# optional; defaults to 2024-10-21
# export AZURE_FOUNDRY_API_VERSION=2024-10-21
EOF
source ~/.lakshmi.env
```

Lakshmi uses Azure Foundry's OpenAI-compatible REST surface. Any deployment that supports `response_format: json_schema` will work; `gpt-4o-mini` is a good cheap default.

### 2. Ask something

```bash
./bin/lakshmi ask "what's my IT exposure?"
```

Or inside the REPL, either form works:

```
₹ › /ask what's my IT exposure?
₹ › what's my IT exposure?
```

You get the **universal verdict format** (F1.6):

- a **verdict line** — 🟢 / 🟡 / 🔴 and a one-sentence headline with inline `[n]` citations,
- **Why** — up to 3 short bullets backing the verdict, each with its own citations,
- **Detail** — the long-form payload (tables, prose); collapses past 20 lines and expands with `more`,
- **Sources** — tier-labelled (1 = official, 2 = broker, 3 = derived) with primary-source URLs,
- **Confidence** — a deterministic 0–99% score derived from source mix + recency (not the model's self-rating),
- **Next** — 1–2 suggested follow-up commands. Type `1` or `2` to run them without retyping.

Follow-ups inherit session context:

```
₹ › what about energy?
```

The last three Q&A pairs are sent to the model so "what about X?" resolves correctly.

### 3. Inspect the reasoning

```
₹ › /why
```

Prints every tool that ran, how long it took, and the token cost of the LLM call. Session-scoped — nothing is persisted.

### Refusals are first-class

If no grounded source supports an answer, Lakshmi says so instead of guessing:

```
🔴  I don't have a source for that.
  • no tool returned data relevant to the question
```

This is by design. Sprint 2 adds the agentic planner loop and more tools (quotes, filings, news); until then, questions outside "holdings + trivial sectors + time" will often refuse.


## Why Kite Connect (and not an MCP or scraper)

- **You are the user.** The access token is issued to your Zerodha account directly — no middleman, no "share your creds with us" server.
- **Read-only by design.** The `Broker` interface intentionally exposes only `Session`, `Logout`, and `Holdings`. Sprint 1 cannot place or cancel orders; that is a compile-time guarantee.
- **Deterministic data.** Holdings, LTP, and historical bars come from the same source Zerodha shows you in Kite — the numbers reconcile exactly.
- **No ToS risk.** Scraping Yahoo/NSE JSON endpoints is fragile and against their terms. Kite Connect is the sanctioned path.

## Security notes

- `http://127.0.0.1:7878/...` is the loopback interface — traffic never leaves your machine. This is the pattern recommended by RFC 8252 ("OAuth 2.0 for Native Apps") and used by the GitHub, Google, AWS, and Stripe CLIs.
- A random 128-bit `state` nonce is generated for every login and validated on the callback (CSRF protection).
- The `KITE_API_SECRET` is read from the environment only at login time and never written to disk.
- The access token lives in your OS keychain under service name `sh.lakshmi`, account `zerodha`. Inspect on macOS with:
  ```bash
  security find-generic-password -s sh.lakshmi -a zerodha
  ```
- `~/.lakshmi/session.json` contains user id + expiry only — **no secrets**.
- If you ever commit your API secret by mistake, rotate it immediately from the Kite Connect developer console.

## Troubleshooting

| Symptom | Likely cause |
|---|---|
| `KITE_API_KEY is not set` | Env var didn't make it to the child process. Check `echo $KITE_API_KEY` in the same shell. |
| `listen on 127.0.0.1:7878 (is the redirect port busy?)` | Another process already bound that port. Free it or set `KITE_REDIRECT_PORT` to something else **and** update your Kite app's redirect URL to match. |
| `zsh: command not found: lakshmi` (exit 127) | The binary isn't on `$PATH`. Either run `./bin/lakshmi` from the repo, or `make install` then add `$GOPATH/bin` to `PATH`. |
| Browser page says "This site can't be reached" after login | The CLI crashed before the redirect arrived. Scroll up in the terminal for the real error. |
| `invalid state in callback` | You followed a stale Kite login URL from a previous attempt. Just run `./bin/lakshmi login` again. |
| macOS prompts for keychain password on every run | Click **Always Allow** once; macOS then trusts the binary until you rebuild. A code-signed release build fixes this permanently. |

## Repo layout

```
cmd/lakshmi/              CLI entry point + subcommand wiring
internal/broker/          Broker interface, session store, keyring token store
internal/broker/zerodha/  Kite Connect client (login flow, holdings)
internal/cache/           File-backed local cache with per-namespace freshness rules
internal/agent/           Grounded question-answering loop (plan/fetch/reason/shape + trace)
internal/shaper/          Universal verdict renderer (F1.6) — pure, theme-able, golden-testable
internal/llm/             Chat-completion client (Azure AI Foundry)
internal/tools/           Tool interface + Sprint 1 tools (holdings, sector, time)
internal/config/          Env-based config loader
internal/paths/           ~/.lakshmi/ layout
internal/portfolio/       Holdings table renderer + shaper adapter
internal/repl/            Bubbletea REPL, dispatcher, history, banner, numbered shortcuts
internal/version/         Build-stamped version string
sprints/                  Per-sprint feature specs and technical plans
```

## Development

```bash
make build      # build ./bin/lakshmi with a git-hash version stamp
make test       # run the full test suite
make tidy       # go mod tidy
make install    # install to $GOPATH/bin for convenience
make clean      # remove ./bin
```

---

🪷 May your alpha be abundant.