# 🪷 lakshmi.sh

> wealth, intelligently.

A conversational, grounded stock-analysis terminal for Indian markets. Single Go binary, local-first, every claim cited.

**Status:** 🚧 Sprint 1 in progress. See [`spec.md`](spec.md) for the product spec and [`sprints/README.md`](sprints/README.md) for the build plan.

## What ships today

| Feature | Status | What it does |
|---|---|---|
| F1.1 REPL shell | ✅ | `₹ ›` prompt, history (↑/↓), `/help`, `/exit`, `:q` |
| F1.2 Zerodha login | ✅ | Browser OAuth via Kite Connect, token in OS keychain |
| F1.3+ | 🔜 | Holdings, config, agent loop, grounded output |

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
internal/config/          Env-based config loader
internal/paths/           ~/.lakshmi/ layout
internal/repl/            Bubbletea REPL, dispatcher, history, banner
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
# 🪷 lakshmi.sh

> wealth, intelligently.

A conversational, grounded stock-analysis terminal for Indian markets. Single Go binary, local-first, every claim cited.

**Status:** 🚧 Sprint 1 in progress. See [`spec.md`](spec.md) for the product spec and [`sprints/README.md`](sprints/README.md) for the build plan.

## Quick start (dev)

```bash
make build      # produces bin/lakshmi
./bin/lakshmi   # launches the REPL — type /help
make test       # run all tests
```

Requires Go 1.22+.

---

🪷 May your alpha be abundant.