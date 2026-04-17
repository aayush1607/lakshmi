# Sprint 1 — Technical Plan

> Scope: F1.1 REPL · F1.2 Zerodha login · F1.3 Portfolio · F1.4 Local store · F1.5 Grounded ask · F1.6 Response shaper
>
> Goal of this doc: turn PM specs into a concrete, build-ready technical blueprint. Not a line-by-line design — enough that any competent Go engineer can sit down and start.

---

## 1. Design principles for Sprint 1

1. **Single binary, zero runtime deps.** No servers, no sidecars, no Docker for v1. The user runs `lakshmi` — that's it.
2. **Everything is a tool call.** The agent loop doesn't special-case portfolio vs. stock questions; it just invokes typed tools. This keeps F1.5 (ask) and F1.3 (`/portfolio`) built on the same primitives.
3. **Grounding is structural, not prompted.** The LLM never produces citations itself. Citations come from which tools were actually called. This is the only way F1.5's "grounded or silent" promise holds.
4. **Pure core, imperative shell.** TA math, formatting, scoring — pure functions. Networking, I/O, LLM calls — isolated at the edges. Easier to test, easier to reason about.
5. **Small files.** Target ≤300 LOC per file. One concept per package.
6. **Determinism where possible.** Same inputs → same output layout (F1.6 criterion #5). LLM non-determinism is contained to the `reason` step; everything around it is deterministic.

---

## 2. Tech stack (confirmed)

| Concern | Choice | Notes |
|---|---|---|
| Language | **Go 1.22+** | Single binary, great concurrency, boring. |
| CLI framework | **cobra** | Subcommands (`lakshmi login`, `lakshmi repl`, …). |
| TUI / REPL | **`charmbracelet/bubbletea` + `bubbles/textinput` + `lipgloss`** | Industry standard for Go TUIs. |
| Line editing / history | **`chzyer/readline`** or Bubbletea's own | Readline is simpler for pure prompt UX; Bubbletea is better if we do rich UI later. **Pick Bubbletea** for Sprint 1 so we don't have to migrate for darshans. |
| Local KV (tokens, small state) | **BadgerDB** | Embedded, Go-native, pure-Go option available. |
| Local SQL/analytics (later sprints) | **DuckDB via go-duckdb** | Deferred until Sprint 2 when we have price history. Sprint 1 uses BadgerDB + JSON blobs only. |
| Secrets | **OS keychain via `zalando/go-keyring`** | macOS Keychain / Linux secret-service / Windows Credential Manager. Falls back to AES-GCM-encrypted file if unavailable. |
| HTTP client | stdlib `net/http` + `hashicorp/go-retryablehttp` | Retry, backoff, request IDs. |
| Config | **YAML** via `go-yaml/yaml` | `~/.lakshmi/config.yaml`. |
| Logging | **`log/slog`** (stdlib, Go 1.21+) | JSON logs in `~/.lakshmi/logs/`. |
| LLM clients | **Azure AI Foundry** (primary, only) via its OpenAI-compatible REST API | Hand-rolled thin client behind an `llm.Client` interface. Anthropic/OpenAI direct can be added later without touching call sites. |
| Zerodha | **Kite Connect v3 REST API** via official `zerodhatech/gokiteconnect` | Well-maintained. |
| Testing | stdlib `testing` + `testify/require` | Golden files for TUI rendering. |

**Total new Go modules added in Sprint 1: ~10.** Comfortably under the <20 target.

---

## 3. Repository layout

```
lakshmi/
├── cmd/
│   └── lakshmi/
│       └── main.go                 # cobra root, dispatches to subcommands
├── internal/
│   ├── app/                        # wiring: builds the agent, tools, store from config
│   │   └── app.go
│   ├── config/                     # load ~/.lakshmi/config.yaml + defaults
│   │   └── config.go
│   ├── paths/                      # resolves ~/.lakshmi/{data,logs,history,config.yaml}
│   │   └── paths.go
│   ├── repl/                       # F1.1 — Bubbletea model, history, slash-command dispatch
│   │   ├── model.go
│   │   ├── history.go
│   │   └── banner.go
│   ├── shaper/                     # F1.6 — renders the universal verdict format
│   │   ├── render.go
│   │   ├── theme.go                # lipgloss styles
│   │   └── golden_test.go
│   ├── agent/                      # F1.5 — 5-phase grounded loop
│   │   ├── agent.go                # Run(ctx, query) -> Answer
│   │   ├── understand.go           # intent classification
│   │   ├── plan.go                 # tool-selection step
│   │   ├── fetch.go                # executes tools in parallel
│   │   ├── reason.go               # LLM synthesis over fetched context
│   │   └── trace.go                # captures tool calls for /why
│   ├── tools/                      # typed tool registry
│   │   ├── registry.go
│   │   ├── portfolio.go            # wraps broker.Holdings() → Tool
│   │   └── types.go                # Tool interface, ToolResult with Sources
│   ├── broker/
│   │   ├── broker.go               # interface Broker { Holdings, Login, … }
│   │   └── zerodha/                # F1.2 + F1.3
│   │       ├── client.go
│   │       ├── login.go
│   │       └── holdings.go
│   ├── store/                      # F1.4
│   │   ├── store.go                # interface + open/close
│   │   ├── kv.go                   # Badger impl for freshness + blobs
│   │   ├── freshness.go            # TTL rules per domain
│   │   └── tokens.go               # keyring wrapper
│   ├── llm/
   │   ├── client.go               # interface Client { Complete, CompleteJSON }
   │   └── azure.go                # Azure AI Foundry implementation
│   └── version/version.go
├── pkg/                            # (nothing public yet)
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

Rule: **only `cmd/` imports `internal/app/`; only `internal/app/` wires packages together.** No package imports another peer package for convenience.

---

## 4. Component design

### 4.1 REPL (F1.1)

```go
// internal/repl/model.go
type Model struct {
    input    textinput.Model
    viewport viewport.Model      // scrollback
    history  *History            // ~/.lakshmi/history (plain file, newline-delimited)
    disp     Dispatcher          // maps "/portfolio" -> handler
    banner   string
}
```

- Bubbletea `Update` handles:
  - `Enter` → push to history, dispatch to `Dispatcher`.
  - `Up`/`Down` → history navigation.
  - `Ctrl-C` → clear current line (don't exit).
  - `Ctrl-D` / `/exit` → `tea.Quit`.
- Dispatcher is a `map[string]Handler` where a handler returns `tea.Cmd`s producing messages (`AnswerMsg`, `ErrorMsg`) so long-running work doesn't block the UI.
- Unknown slash commands → `AnswerMsg{Kind: UnknownCommand}` rendered by shaper.
- Free-form text (no leading `/`) routes to the `agent.Run` handler (F1.5).

**History file format:** `~/.lakshmi/history`, one line per entry, de-duplicated on load, capped at 1000 entries. No timestamps for v1 — keeps it trivially diffable.

### 4.2 Paths & config

```go
// internal/paths/paths.go
func Home() string              // ~/.lakshmi, honours $LAKSHMI_HOME
func Data() string               // ~/.lakshmi/data
func Logs() string               // ~/.lakshmi/logs
func HistoryFile() string        // ~/.lakshmi/history
func ConfigFile() string         // ~/.lakshmi/config.yaml
```

First-run behaviour: if `~/.lakshmi/` doesn't exist, create it with mode `0700` and drop a default `config.yaml`.

`config.yaml` (v1):
```yaml
llm:
  provider: azure-foundry
  endpoint: https://<your-resource>.openai.azure.com   # or the Foundry project endpoint
  deployment: gpt-4o-mini                              # the deployed model name
  api_version: "2024-10-21"                            # Azure OpenAI API version
  api_key_env: AZURE_FOUNDRY_KEY                       # never stored in config
  # Optional, if using Entra ID / managed identity instead of key:
  # auth: entra
broker:
  provider: zerodha
  api_key: <kite app id>    # public identifier; not secret
ui:
  theme: auto               # auto | light | dark
  unicode: true             # false disables emoji / box-drawing
telemetry: false            # always off in v1
```

### 4.3 Store (F1.4)

```go
// internal/store/store.go
type Store interface {
    Get(ns, key string, out any) (fresh bool, err error)
    Put(ns, key string, val any, ttl time.Duration) error
    Bust(ns, key string) error
    Stats() (Stats, error)
    Clear() error
    Close() error
}
```

- Backed by BadgerDB. Namespaces: `holdings`, `quotes`, `fundamentals`, `news`, `prices` (Sprint 2).
- Stored value = `{ fetched_at, ttl, payload }`. `Get` returns `fresh=true` only if `now < fetched_at+ttl` **and** (for market-hours sensitive namespaces) it's currently market hours.
- Freshness rules live in `freshness.go` — **single source of truth**, tested independently:
  ```go
  var Rules = map[string]FreshnessRule{
      "holdings":     {Open: 2 * time.Minute,  Closed: 24 * time.Hour},
      "quotes":       {Open: 60 * time.Second, Closed: 24 * time.Hour},
      "fundamentals": {Open: 24 * time.Hour,   Closed: 24 * time.Hour},
  }
  ```
- `--fresh` flag bypasses `Get` — always calls the upstream, then `Put`s.
- Tokens/secrets do **not** go through this store; they go to `tokens.go` (keyring).

### 4.4 Broker / Zerodha (F1.2 + F1.3)

```go
// internal/broker/broker.go
type Broker interface {
    Login(ctx context.Context) (Session, error)     // launches browser flow
    Session() (Session, error)                      // current, from keyring
    Logout() error
    Holdings(ctx context.Context) ([]Holding, error)
}

type Session struct {
    UserID    string
    UserName  string
    Expiry    time.Time
    // access_token is NOT in this struct — only inside zerodha pkg, loaded on demand
}
```

**Login flow (F1.2):**
1. User triggers `lakshmi login` or `/login zerodha`.
2. We build the Kite login URL with our registered `api_key` + a random `state`.
3. Open it via `browser.OpenURL` (`pkg/browser`).
4. Spin up a localhost HTTP server on an ephemeral port listening for the redirect. Our registered redirect URI points to `http://127.0.0.1:<port>/callback` (Kite allows configuring this).
5. On callback, grab `request_token`, exchange for `access_token` via the Kite `/session/token` endpoint (signed with our `api_secret`). Store token in keyring under service name `lakshmi.zerodha`.
6. Fetch `/user/profile` for display name + user ID. Persist `Session` metadata (no token) in store namespace `session` for `/login status`.

**Secrets:**
- `api_secret` for our Kite app: loaded from `KITE_API_SECRET` env var (required only at build-time of the redeemer step). **We never ship a secret embedded in the binary.** For open-source distribution we'll document: users register their own Kite Connect app (one-time, free, 10-minute process) and put their key + secret in their own env. This sidesteps the entire "shipping secrets" problem.
- `access_token` — OS keychain via `go-keyring`. Fallback: AES-GCM file with key derived from a machine-specific stable value (`/etc/machine-id` on Linux, hardware UUID on mac) — this is not strong security, but it means a casual file copy doesn't leak the token.
- `/login status` reads Session metadata; if expiry < now, reports expired.

**Holdings (F1.3):**
- Thin wrapper over `kiteconnect.Holdings()`. Maps Kite's struct into our own `Holding` type so we're not tied to vendor shapes.
- Sets source metadata: `Source{Name: "Zerodha Kite", Tier: 2, URL: "kite.zerodha.com", FetchedAt: now}`.
- Read-only guarantee: the `Broker` interface has **no** mutating methods in Sprint 1. Enforced by code review + a unit test asserting `reflect.TypeOf` has no `Place*`/`Cancel*` methods.

### 4.5 Tools (the agent's capabilities)

```go
// internal/tools/types.go
type Tool interface {
    Name() string
    Schema() jsonschema.Schema        // for LLM tool-calling
    Call(ctx context.Context, args json.RawMessage) (Result, error)
}

type Result struct {
    Data    any          // structured, JSON-marshalable
    Summary string       // human-readable one-liner for logs / traces
    Sources []Source     // every fact in Data must be attributable to one
}

type Source struct {
    Name      string
    URL       string
    Tier      int        // 1, 2, 3
    FetchedAt time.Time
}
```

Sprint 1 tools:

| Tool | Backed by | Produces |
|---|---|---|
| `portfolio_holdings` | `broker.Holdings()` via store | `[]Holding` + Tier 2 Zerodha source |
| `portfolio_summary` | pure function over holdings | totals, sector breakdown (basic mapping hardcoded or via sector lookup in Sprint 2) |
| `time_now` | stdlib | trivial, but LLM uses it for "is it market hours?" reasoning |

That's enough for Sprint 1. `/portfolio` is literally the `portfolio_holdings` tool rendered through the shaper. F1.5's `ask` uses all three.

### 4.6 Agent loop (F1.5)

```go
// internal/agent/agent.go
func (a *Agent) Run(ctx context.Context, query string) (Answer, error) {
    trace := NewTrace()

    intent := a.Understand(ctx, query, trace)                 // LLM classification (cheap)
    plan   := a.Plan(intent, a.tools, trace)                  // may be deterministic for known intents
    fetched:= a.Fetch(ctx, plan, trace)                       // parallel tool calls
    reply  := a.Reason(ctx, query, fetched, trace)            // LLM synthesis, RAG-pure
    return a.Shape(query, reply, fetched, trace), nil         // feeds into F1.6
}
```

- **UNDERSTAND**: single LLM call, ~200 tokens in, structured output `{intent, entities}`. For `/portfolio`, we skip this — intent is known.
- **PLAN**: deterministic for known commands; LLM-driven only for free-form `ask`. Plan is just `[]ToolCall{...}`. Shown to user as a one-liner: `⟳ plan: read holdings · classify sectors · summarise`.
- **FETCH**: `errgroup.Group`, all tool calls in parallel, each capturing its `Sources`. A tool failure degrades gracefully — the rest still run, and the reason step is told which tools failed.
- **REASON**: LLM is given **only** the fetched results, with an instruction to cite via source index. The prompt contains: `system` (grounding rules + refusal policy), `user` (query), `context` (serialised tool results with `[1] source: …` indices). Output is structured JSON: `{verdict, why[], detail, confidence, next_actions[]}`. **The LLM never invents sources** — it picks indices that already exist.
- **SHAPE**: attaches the actual `[]Source` from fetched results to the answer based on which indices REASON cited. If an index wasn't cited, its source is dropped from the final sources block.

**Grounding enforcement:**
- Post-process: parse REASON output; every `[n]` citation must resolve to a fetched source, else the answer is rejected and we retry once with a stricter system prompt. After two failures, we return a "I don't have a grounded answer for that" shaped response. This is a deterministic safeguard — not a vibe.

**`/why` trace:**
- `Trace` is a `[]TraceEvent` appended at each phase. On `/why`, the shaper renders the last trace — tool names, input args, truncated outputs, timings. Persisted in memory per REPL session, not on disk.

**Token budget:**
- Understand + Reason together should stay under ~15k input tokens typical. Fetched results are serialised compactly (no pretty JSON) with per-tool size caps; large results are summarised before insertion.

**LLM client (Azure AI Foundry specifics):**

```go
// internal/llm/client.go
type Client interface {
    Complete(ctx context.Context, req Request) (Response, error)
    // CompleteJSON enforces a JSON schema on the response (used by Understand + Reason).
    CompleteJSON(ctx context.Context, req Request, schema any) (json.RawMessage, error)
}

type Request struct {
    System    string
    Messages  []Message           // role: user | assistant
    MaxTokens int
    Temperature float32
    Tools     []ToolSpec          // tool-calling for the Plan step (free-form ask only)
}
```

- **Endpoint shape:** `POST {endpoint}/openai/deployments/{deployment}/chat/completions?api-version={api_version}`. This is Azure's OpenAI-compatible path; Foundry projects expose the same shape with either a project-level or deployment-level URL.
- **Auth:** header `api-key: $AZURE_FOUNDRY_KEY` for v1. If the user sets `auth: entra` in config, we swap to `Authorization: Bearer <token>` using `azidentity.DefaultAzureCredential` — useful for users on managed identity, but not required in v1.
- **Model pinning:** the deployment name is the pin. We do **not** hardcode a model family in our code; the user's Foundry deployment dictates which model runs. This means our prompts must be family-agnostic (avoid Anthropic-only XML tags, avoid OpenAI-only tool syntax quirks — stick to plain Chat Completions + JSON mode).
- **JSON mode:** Azure OpenAI supports `response_format: {"type": "json_object"}` and (on newer API versions) `json_schema`. Use `json_schema` where available for strict validation of the Reason step's output; fall back to `json_object` + client-side schema validation.
- **Rate limits / retries:** Foundry deployments have TPM/RPM quotas per deployment. Client honours `Retry-After` on 429/503 via `retryablehttp` with jitter. On repeated 429, surface a shaped Refusal answer pointing the user to check their Foundry deployment quota.
- **Observability:** request ID (`x-ms-request-id`) and model usage (`prompt_tokens`, `completion_tokens`) captured into the `Trace` so `/why` can show cost/latency breakdown.
- **No streaming in v1.** Final answer only. Streaming is a Sprint-3-or-later polish.
- **Interface-first:** `llm.Client` is the only abstraction; Anthropic / OpenAI direct / local Llama can be added later as new implementations without touching the agent.

### 4.7 Shaper (F1.6)

```go
// internal/shaper/render.go
type Answer struct {
    Verdict      Verdict            // {Colour, Text}
    Why          []string
    Detail       string             // may be long; collapse > N lines
    Sources      []Source
    Confidence   int                // 0..100
    NextActions  []string           // slash-commands
    Kind         AnswerKind         // Normal | Informational | Refusal | UnknownCommand
}

func Render(a Answer, theme Theme) string
```

- Pure function. No I/O. Easy to golden-test.
- Confidence computed deterministically from:
  - `sourceCount` (≥3: +20, ≥5: +30)
  - `tier1Count`, `tier2Count`, `tier3Count` weightings
  - `conflicts` (detected during REASON and surfaced as a field)
  - `recency` (max age of sources)
  Clamped 0–100. Same inputs → same number.
- Detail is collapsed via a `[... N more lines — type 'more' to expand]` line; the REPL model holds the full string and swaps on `more`.
- Theme: lipgloss styles for light/dark/auto. `unicode: false` in config replaces 🟢🟡🔴 with `[OK]`/`[!!]`/`[NO]` and box-drawing with ASCII.
- **Golden tests**: `golden_test.go` ships ~10 canonical `Answer` fixtures and asserts the rendered output matches stored `.golden` files. CI flag `-update` regenerates them.

---

## 5. Cross-cutting concerns

### Error handling
- All user-facing errors go through the shaper as a `Refusal`-kind Answer, never raw stack traces.
- Internal errors logged via `slog` with a request ID. In debug mode (`LAKSHMI_DEBUG=1`), stack traces surface in `/why`.

### Context & cancellation
- Every network-touching function takes `context.Context`.
- REPL wires `Ctrl-C` (during a long-running command, not just at prompt) to cancel the context — crucial for LLM calls that can hang.

### Concurrency
- Only the FETCH phase runs in parallel. REPL is single-threaded in Bubbletea's event loop. Store uses Badger's own locking.

### Logging
- `slog` JSON handler → `~/.lakshmi/logs/lakshmi-YYYY-MM-DD.log`, rotated daily (simple time-based; no external lib).
- **Never** log tokens, `Authorization` headers, or API keys. A `redact.Middleware` wraps the HTTP client to strip these from logged request dumps.

### Observability
- No external telemetry in v1. `telemetry: false` is hard-coded-ish — flipping to `true` is a no-op stub for v1.5.

### Performance budgets
| Operation | Budget |
|---|---|
| REPL startup to prompt | 500 ms (F1.1 #1) |
| Cached `/portfolio` render | 200 ms (F1.4 #1) |
| Fresh `/portfolio` fetch + render | 3 s (F1.3 #1) |
| `ask` simple portfolio question | 10 s (F1.5 #5) |

Sprint 1 exit: run `go test -bench=.` on key paths and confirm budgets in a Makefile target `make perf`.

---

## 6. Testing strategy

| Layer | How we test | Examples |
|---|---|---|
| Pure functions (shaper, freshness, confidence) | Table-driven + golden files | `TestRender_VerdictColours`, `TestFreshness_MarketHours` |
| Store | Ephemeral Badger dir in `t.TempDir()` | `TestStore_TTL`, `TestStore_Clear` |
| Broker | Fake `Broker` implementation | Agent tests don't hit Zerodha. |
| Zerodha client | `httptest.Server` with recorded responses | Login callback, holdings endpoint. |
| Agent loop | Fake LLM (returns canned JSON) + fake tools | Verify grounding enforcement, trace capture, refusal path. |
| REPL | Bubbletea's `teatest` package | Send keypresses, assert model state. |
| End-to-end | Scripted `expect`-style test invoking the binary | `make e2e` — optional on CI. |

**Coverage target for Sprint 1:** 70%+ on `internal/shaper`, `internal/store`, `internal/agent`. REPL and broker can be lower (more integration-shaped).

---

## 7. Build, release, ergonomics

- `Makefile` targets: `build`, `test`, `lint` (golangci-lint), `perf`, `run`, `clean`, `install` (to `$GOBIN`).
- `goreleaser` config — macOS (arm64+amd64) + Linux (amd64+arm64). Not Windows for v1.
- Version stamp via `-ldflags "-X internal/version.Version=$(git describe)"`.
- No installer / brew tap in Sprint 1 — binary tarball + `go install github.com/…/cmd/lakshmi@latest` is enough for beta.

---

## 8. Open questions (resolve before coding)

1. **Kite Connect app: do we ship a shared one, or require users to register their own?**
   - Recommendation: **require their own**. Sidesteps secret-shipping, per-user rate limits are isolated, and Kite's free tier allows this trivially. Document a 10-minute setup.
2. **LLM key: BYO or hosted?**
   - Recommendation: **BYO Azure AI Foundry** for v1. User supplies endpoint + deployment name + API key (or Entra ID token) via env. Zero cost to us, zero vendor lock-in in our code (the `llm.Client` interface means swapping to direct Anthropic/OpenAI later is a one-file change).
3. **DuckDB in Sprint 1?**
   - Recommendation: **no.** Sprint 1 has no tabular price data. Introduce in Sprint 2 when F2.1 lands. Keeps dep count low.
4. **Readline vs Bubbletea for Sprint 1?**
   - Recommendation: **Bubbletea.** Slightly more work now, but avoids rewriting for F3.3 darshans.

---

## 9. Sprint 1 build order (12 working days, sequenced)

This is a suggested order, not a schedule. Each step ends with something runnable.

| Day | Ship | Feature |
|---|---|---|
| 1 | `lakshmi version` + paths + config scaffolding | foundation |
| 2 | Bubbletea REPL with banner + echo + `/help` + `/exit` | F1.1 (partial) |
| 3 | History file, up/down recall, Ctrl-C semantics | F1.1 complete |
| 4 | Store interface + Badger impl + `/cache status` + `/cache clear` | F1.4 |
| 5 | Keyring wrapper + `lakshmi login` browser flow (happy path) | F1.2 (partial) |
| 6 | Login error paths, `/login status`, `/logout` | F1.2 complete |
| 7 | `broker.Holdings` + `/portfolio` raw rendering (table only, no shaper) | F1.3 (partial) |
| 8 | Shaper skeleton: `Answer` struct + `Render` for table/holdings | F1.6 (partial) |
| 9 | Tool registry + `portfolio_holdings` tool + LLM client interface | F1.5 (partial) |
| 10 | Agent loop (understand → plan → fetch → reason) with fake LLM test | F1.5 core |
| 11 | Grounding enforcement, `/why` trace, refusal path | F1.5 complete |
| 12 | Shaper polish (confidence, next, theme), golden tests, `/portfolio` re-rendered through shaper | F1.6 complete |

**Exit criterion:** a user can run `lakshmi`, log in to Zerodha, run `/portfolio`, ask "what's my IT exposure?", and get a verdict-formatted answer with working `/why`. All within budgets listed in §5.

---

## 10. What Sprint 1 deliberately does *not* build

- Any market data beyond what Zerodha's holdings endpoint returns (no prices, no OHLCV).
- Any real analysis (TA/FA/rebalance — Sprint 2).
- Any scheduled/background work (alerts, mantras — Sprint 3).
- Any rendering of charts or live dashboards.
- Sector/industry mapping beyond a trivial lookup — proper classification arrives with F2.x.

Keeping this list short is how we ship Sprint 1 on time.
