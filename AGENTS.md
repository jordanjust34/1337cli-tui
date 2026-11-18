# AGENTS.md

Guide for coding agents working on this repo: how it works, where things live, and how to change it safely.

## What this is

Two Go binaries that turn "find a torrent on 1337x" into "it's downloading into my Jellyfin library":

- **`1337cli`**: scriptable CLI. Searches 1337x, extracts magnet links and adds them to a qBittorrent server.
- **`1337tui`**: interactive front end. Search, pick results and a category, confirm, and it sends them to qBittorrent itself.

Product boundary: the programs only find torrents, resolve magnet links and hand them to qBittorrent. They never download torrent content themselves and don't show download progress; qBittorrent (and its WebUI) owns the transfer.

Both are thin front ends over the same library, `internal/core`, and each works without the other installed. The CLI owns flags, table/JSON output and exit codes. The TUI owns selection, confirmation and rendering.

The user develops on a Mac. Both tools run on a separate Linux mini-PC that also hosts qBittorrent and Jellyfin. **Its addresses, ports, paths and how each service runs are in [`SERVER.md`](SERVER.md); read it before debugging anything on the server, and ask the user about fields marked `TODO` instead of guessing.** It's gitignored, so it only exists in the user's checkout on the Mac; if it's missing, ask the user. qBittorrent categories (`movies`, `tv`, ...) have save paths pointing at Jellyfin library folders, so the category chosen decides which library a download lands in.

## How it works

```
 ┌──────── 1337cli (main.go, cmd/) ────────┐   ┌──────────── 1337tui (tui/) ───────────┐
 │ root.go   search, -g/-i/--best, JSON    │   │ search → pick results → pick category │
 │ qbit.go   `get`, qBittorrent flags      │   │ → confirm → send → result screen      │
 │ categories.go  `categories` [--json]    │   │ model.go: screens, `backend` interface│
 │ config.go flags default to env/config   │   │ main.go: config → core.New → run      │
 └───────────────────┬─────────────────────┘   └───────────────────┬───────────────────┘
                     └──────────────────┬───────────────────────────┘
                                        ▼
 internal/core: Config (env + ~/.config/1337cli/config), Service.Search (paging, retry,
   warnings), Service.Categories, Service.Submit (page → magnet → add, per-item Outcome),
   Explain (Cloudflare hints). Never prints; front ends present results.
           ┌────────────────────────────┴─────────────────────────┐
           ▼                                                      ▼
 internal/scraper                              internal/qbit
 client.go: GET pages directly or via          client.go: Web API v2 (login,
   FlareSolverr (Cloudflare)                     torrents/add, categories,
 parse.go: search table → []Result,              app/version), cookie session,
   detail page → magnet, challenge detection     CSRF headers
           ▼                                          ▼
   1337x.to ← FlareSolverr (Docker, :8191)      qBittorrent WebUI (:8080)
                                                      ▼
                                          category save path → Jellyfin library
```

### Request flow, step by step

1. **Search** (`1337cli -s ubuntu -n 45`, or the TUI's search box): `core.Service.Search` calls `scraper.Client.Search(query, page)` for page after page, until it has `--limit` results or gets a short page (fewer than 20 = last page). An empty page in the middle is retried once, because 1337x sometimes serves one. A later page that fails keeps the results already collected and reports a warning through `Service.Warn` (the CLI prints it to stderr; the TUI drops it).
2. **Fetch**: `scraper.Client.get` either does a plain HTTP GET, sending `--cookie`/`--user-agent` if set, or POSTs `{"cmd":"request.get"}` to FlareSolverr and uses `solution.response`. Cloudflare's 403/503 bodies are passed to the parser instead of being treated as HTTP errors, so the parser can recognise the challenge page.
3. **Parse**: `ParseSearch` reads `table.table-list tbody tr`. Name and link come from `td.coll-1 a[href^="/torrent/"]`, seeders from `coll-2`, leechers from `coll-3`, date from `coll-date`, size from `coll-4` (its own text only, since a hidden `<span>` repeats the seeder count), uploader from `coll-5`. The title "Just a moment..." means `ErrChallenge`, which `core.Service.Explain` turns into remediation advice.
4. **Send** (`get`, `-g`, `--best`, `-i`, or confirming in the TUI): the CLI picks targets in `chooseTargets` (`cmd/root.go`): `-g` via `parseSelection` (`cmd/select.go`), `--best` via `core.Best`, `-i` via the stdin prompt in `cmd/prompt.go`. When the root command will send, it calls `core.Service.Ping` *before* searching, so a bad qBittorrent login fails without waiting for FlareSolverr. `get -` reads targets from stdin (`expandTargets`). Then `core.Service.Submit` turns each target into a magnet link (`Magnet()` fetches the detail page and takes the first `a[href^="magnet:"]`; anything that already starts with `magnet:` is used as is). It logs in once, then adds each torrent and returns one `Outcome` per target. One failure doesn't stop the rest. The CLI's `sendToQbit` prints a line per outcome and exits non-zero with a count of failures; the TUI shows them on its result screen and exits non-zero if any submission failed.
5. **qBittorrent specifics** (`internal/qbit`):
   - Every request sends `Referer`/`Origin` set to the base URL, because of qBittorrent's CSRF check.
   - Login succeeds with "Ok." (4.x) or `204` (5.x). "Fails." or `401` means failure.
   - `paused` and `stopped` are both sent, because 5.x renamed the field.
   - **`autoTMM=true` is sent whenever a category is set without `--save-path`.** Without it, qBittorrent ignores the category's save path and the download misses the Jellyfin library.
   - `409` means the torrent is already added.
6. **TUI** (`tui/`): Bubble Tea (`charmbracelet/bubbletea` v1). It moves through the screens `search → searching → results → category → confirm → submitting → done`. The model holds a `backend` interface (`Search`, `Categories`, `Submit`, `Ping`) that `*core.Service` implements and tests replace with a recorder. Searches, the category list, the startup `Ping` (its error is shown on the search screen) and the submission run as background `tea.Cmd`s. `b` on the results screen selects the top-seeded row (`core.Best`). Each search has a number (`seq`), and results from cancelled or older searches are dropped. Nothing is submitted until the user confirms; leaving the confirm screen sends nothing. Picking "none" uses `QBIT_CATEGORY` if set, and `QBIT_SAVE_PATH` applies as in the CLI. If nothing is selected, the row under the cursor is used.

## Layout

| Path | Purpose |
|---|---|
| `main.go` | Entry point for `1337cli`. Prints the error and exits 1 (Cobra's own error printing is turned off). |
| `cmd/root.go` | Root command, the search and connection flags, table output, `--json`, `-g`/`--best`/`-i`, `checkSendFlags`, `chooseTargets`. |
| `cmd/select.go` | `parseSelection`: `-g` and prompt input (numbers, ranges, `all`). |
| `cmd/prompt.go` | `-i` prompt (`pickTargets`), terminal check. |
| `cmd/qbit.go` | `get` subcommand, qBittorrent flags (persistent, so they work on every command), `sendToQbit` (prints outcomes, sets the exit status). |
| `cmd/categories.go` | `categories` subcommand (table or `--json`). |
| `cmd/config.go` | Calls `core.LoadConfig` from a package-level `var`, so the env is filled before any `init()` reads it; `newService` builds a `core.Service` from the flags. |
| `internal/core/` | Shared library for both binaries: config loading, search paging, categories, submission, Cloudflare hints. Prints nothing. |
| `internal/scraper/` | 1337x HTTP client and HTML parsing. `testdata/` holds real or realistic pages. |
| `internal/qbit/` | Minimal qBittorrent Web API client. |
| `tui/` | `1337tui` (package `main`): `model.go` has the screens, keys and the `backend` interface, `main.go` loads config, builds a `core.Service` and starts the TUI. |
| `deploy/` | Shipped to the server: `install.sh`, `config.example`, `docker-compose.yml` (FlareSolverr, bound to 127.0.0.1). |
| `Makefile` | `make` builds both binaries, `make test` runs the tests, `make dist` builds the server package. |
| `.claude/skills/ship/` | The `/ship` skill: package, verify, copy to the server. |
| `README.md` | User-facing install and usage guide. Update it when flags, config keys or TUI keys change. |
| `docs/reqs/` | Product and software requirements (LS-xx IDs and acceptance checks). Update the status table when a requirement is implemented. |
| `docs/plans/` | Agreed plans for upcoming work, with subtasks and dependencies. |

## Configuration

Precedence: **flag > environment variable > config file**. Both binaries read the same file and env vars (`core.LoadConfig`, `core.FromEnv`); only the CLI has flags. The config file is `KEY=value` lines (`export` and quotes are allowed), at `os.UserConfigDir()/1337cli/config`. That's `~/.config/1337cli/config` on Linux and `~/Library/Application Support/1337cli/config` on macOS. `LEET_CONFIG` overrides the path.

| Flag | Env / config key | Notes |
|---|---|---|
| `-s, --search` | | Extra positional words are appended, so quotes are optional. |
| `-n, --limit` | | 0 = one page. More than 20 fetches more pages. |
| `-p, --page` | | Start page. |
| `--json` | | Machine output for search and `categories`. |
| `-g, --get` | | Results to send: numbers, ranges or `all`, e.g. `1,3`, `2-5`. |
| `--best` | | Send the result with the most seeders. |
| `-i, --interactive` | | After the table, prompt (on stderr) for result numbers, then a category if none is set. Needs a terminal on stdin; opt-in only, never automatic. |
| `get -` | | Read page URLs or magnets from stdin, one per line (`#` comments and blank lines skipped). |
| `-m, --mirror` | `LEET_MIRROR` | Default `https://1337x.to`. |
| `--flaresolverr` | `FLARESOLVERR_URL` | `/v1` is appended if missing. |
| `--cookie`, `--user-agent` | `LEET_COOKIE`, `LEET_USER_AGENT` | Alternative to FlareSolverr. Both must come from the same browser. |
| `--qbit-url/-user/-pass` | `QBIT_URL`, `QBIT_USER`, `QBIT_PASS` | An empty user skips login. |
| `-c, --category` | `QBIT_CATEGORY` | The category's save path decides the Jellyfin library. |
| `--save-path` | `QBIT_SAVE_PATH` | Overrides the category path (autoTMM is then off). |
| `--paused` | | |
| `1337tui -n` | | Results fetched per TUI search (default 20). |

`--json`, `-g`, `--best` and `-i` are mutually exclusive (`checkSendFlags`).

**Not implemented yet:** `QBIT_FOLDERS` (plain server folders offered next to categories, sent as a save path) is commented out in `config.example` and specified as LS-13 in `docs/reqs/SOFTWARE_REQUIREMENTS.md`. Neither binary reads it. Implement it in `core` (e.g. a destination list that merges categories and folders) so both front ends get it.

## Making changes

General rules:

- Shared behaviour (anything that talks to 1337x or qBittorrent, or decides what gets sent) goes in `internal/core`, so both binaries get it. Front ends only import `core` (it re-exports `Result`, `Category`, `AddOptions`), not `internal/scraper` or `internal/qbit`. `core` must not print or exit; return errors, outcomes, or call `Service.Warn`.
- Keep front-end concerns in the front end: flags, table/JSON output and exit codes in `cmd/`; selection, confirmation and rendering in `tui/`.
- Match the existing style: small packages, errors that tell the user what to do next, a comment explaining *why* something non-obvious is done (CSRF headers, autoTMM, retry on empty page).
- Run `gofmt -l .`, `go vet ./...` and `make test` before calling anything done. Behaviour that touches 1337x or qBittorrent also needs a live check (see Testing).

### Add a flag

- Search or connection flags: `init()` in `cmd/root.go`. qBittorrent flags: `cmd/qbit.go` (persistent). Default from `os.Getenv("KEY")` if it should be configurable, and add the key to `deploy/config.example` and to the table above.
- If it's a setting the library uses, add a field to `core.Config`, read it in `core.FromEnv` (that's how the TUI gets it), and pass the flag value in `newService` in `cmd/config.go`.
- If the TUI user should be able to change it per submission, add it to the model, `addOptions()` and the confirm screen, and to the recorder assertions in `tui_test.go`.

### Add a subcommand

Create `cmd/<name>.go` with a `cobra.Command`, and register it in that file's `init()` with `rootCmd.AddCommand`. The root command uses `cobra.ArbitraryArgs` so that `-s two words` works. Keep that, or unquoted multi-word searches break once subcommands exist. Test it end to end in `cmd/root_test.go`: `run(t, args...)` resets every flag first (`resetFlags`, since flag values live in package variables), `fakeSite`/`fakeQbit` stand in for 1337x and qBittorrent, and `stdout` captures output.

### Change JSON output (the CLI's script contract)

`scraper.Result` and `qbit.Category` (re-exported as `core.Result` and `core.Category`) are what `--json` encodes. Scripts parse those field names, so only add fields; never rename or remove one. `TestResultJSONFields` in `internal/core/core_test.go` pins the names and `TestRootJSON` in `cmd/root_test.go` checks the CLI output, so update them when adding a field. The TUI uses the same structs directly, so it has no separate copy to keep in sync.

### 1337x changed its HTML

Symptoms: searches return nothing, or columns come back empty or zero, with no Cloudflare error.

1. Save a live page through FlareSolverr:
   ```sh
   curl -s localhost:8191/v1 -H 'Content-Type: application/json' \
     -d '{"cmd":"request.get","url":"https://1337x.to/search/ubuntu/1/","maxTimeout":60000}' \
     | python3 -c 'import json,sys;print(json.load(sys.stdin)["solution"]["response"])' > /tmp/page.html
   ```
2. Update the selectors in `internal/scraper/parse.go`.
3. Replace or add fixtures in `internal/scraper/testdata/` (trimmed to a few rows), and update the expected values in `parse_test.go`.
4. Do the same for detail pages and `ParseMagnet` (`testdata/detail.html`). Download-button class names are randomised, so keep matching on the `magnet:` scheme.

If every mirror starts returning challenges even through FlareSolverr, update the FlareSolverr image (`docker compose pull && docker compose up -d`) before touching the code.

### Add a qBittorrent API call

Add a method in `internal/qbit/client.go` that calls `Login()`, then `c.post` or `c.do`, so it gets the CSRF headers and status-code mapping. Test it against `httptest` in `client_test.go`; the fake server there checks the Referer and session cookie. Check field names against both 4.x and 5.x if the API changed between them.

### Add a TUI screen or key

Add a `screen` constant, an `updateX` and a `viewX` in `tui/model.go`, and wire them into `Update` and `View`. Put the new key in that screen's help line. Long-running work (anything calling the `backend`) must be a `tea.Cmd` that returns a message, never a blocking call inside `Update`. Extend the key-driven tests in `tui_test.go` (`send(m, key(...))`; `confirm` runs the submit command against the `recorder` backend). If the screen needs a new library call, add it to `core.Service`, the `backend` interface and `recorder`.

### Add a binary or a server-side file

Add it to the `dist` loop in the `Makefile` and to `deploy/install.sh`, then check `tar tzf dist/1337cli.tar.gz` (the `/ship` skill checks the exact file list, so update that list in its `SKILL.md` too).

## Testing

```sh
make test          # unit tests: parser fixtures, qbit client vs httptest, core config/paging/retry/submission vs httptest, CLI commands end to end vs fake 1337x + qBittorrent, -g/-i/get - parsing, TUI flow vs a recording backend
```

Live checks, for changes that touch real services:

- **FlareSolverr:** `docker run -d --name flaresolverr -p 8191:8191 ghcr.io/flaresolverr/flaresolverr`, then `FLARESOLVERR_URL=http://localhost:8191 ./1337cli -s ubuntu -n 5`. A page takes about 12 s.
- **qBittorrent:** `docker run -d --name qbit-test -p 127.0.0.1:18080:18080 -e WEBUI_PORT=18080 -e PUID=1000 -e PGID=1000 lscr.io/linuxserver/qbittorrent`. The temporary admin password is in `docker logs qbit-test`. Wait for the WebUI before logging in. Create categories via `/api/v2/torrents/createCategory`, and create their folders inside the container owned by `abc`. Use small, legal torrents (Linux ISOs) and delete them afterwards with `deleteFiles=true`. To exercise `-g`/`--best`/`-i`/the TUI without touching 1337x, point `LEET_MIRROR` at a tiny local server that serves `internal/scraper/testdata/search.html` for `/search/...` and `detail.html` (an Ubuntu ISO magnet) for `/torrent/...`, and add with `--paused`. Repeated wrong-password logins can get the test client IP-banned by qBittorrent.
- **TUI:** drive it in `tmux` (`tmux new-session -d -x 120 -y 35 ...`, `send-keys`, `capture-pane -p`). To prove it runs standalone, build it into an empty directory, run it with a `PATH` that has no `1337cli`, and point `LEET_CONFIG` at a config with `LEET_MIRROR`/`QBIT_URL` for the test services.
- **Install script:** `docker run --rm -v "$PWD/dist/1337cli.tar.gz:/tmp/r.tgz:ro" ubuntu:24.04 sh -c 'cd /root && tar xzf /tmp/r.tgz && ./1337cli/install.sh'`.

Clean up containers, networks and test torrents when done. Leave the user's own `flaresolverr` container alone.

## Pitfalls already hit

- **qBittorrent Host-header check:** if the URL's port differs from qBittorrent's own WebUI port (a remapped Docker port), every request gets `401` and the log says "Invalid Host header, port mismatch". Map the same port on both sides.
- **Category ignored:** without `autoTMM`, qBittorrent uses its default folder instead of the category's. That's already handled; don't remove it.
- **Folder permissions:** qBittorrent shows `error` if it can't write to the category's folder. Check the folder's owner before suspecting the code.
- **Docker group:** `permission denied ... docker.sock` on the server means the user isn't in the `docker` group. `install.sh` falls back to `sudo`.
- **Mirrors:** some 1337x mirror domains redirect to spam or ad sites. Don't change the default mirror without checking it serves the real site.
- **`scp` without `:`** writes a local file named `user@host`. Those are gitignored (`*@*`), but they mean the copy never happened.

## Packaging and shipping to the server

Use the `ship` skill (`.claude/skills/ship/SKILL.md`, invoked as `/ship` or `/ship user@server-ip`). It covers building `dist/1337cli.tar.gz` with `make dist`, verifying its contents, copying it with `scp`, and the install steps on the server. Agents without skill support should read that file and follow it directly.

Key rules, in case you're working outside the skill:

- "Compress the project" means `make dist`, never a hand-rolled `tar`/`zip`. On macOS a plain `tar` adds headers that make GNU tar on the server print warnings.
- Only `config.example` ships. Never package a real config (`QBIT_PASS`, cookies, ...).
- Copying to the server is outward-facing: only do it when the user asks or gives a target, and always use `scp ... user@host:~` with the colon.
- New binaries or server-side files go in the `Makefile` `dist` target and `deploy/install.sh`.
