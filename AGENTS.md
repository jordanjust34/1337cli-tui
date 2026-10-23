# AGENTS.md

Guide for coding agents working on this repo: how it works, where things live, and how to change it safely.

## What this is

Two Go binaries that turn "find a torrent on 1337x" into "it's downloading into my Jellyfin library":

- **`1337cli`**: scriptable CLI. Searches 1337x, extracts magnet links and adds them to a qBittorrent server.
- **`1337tui`**: interactive front end. It never talks to 1337x or qBittorrent itself. It runs `1337cli`, reads its JSON, lets the user choose, then builds a `1337cli get ...` command and runs it after the TUI exits.

The user develops on a Mac. Both tools run on a separate Linux mini-PC that also hosts qBittorrent and Jellyfin. **Its addresses, ports, paths and how each service runs are in [`SERVER.md`](SERVER.md); read it before debugging anything on the server, and ask the user about fields marked `TODO` instead of guessing.** It's gitignored, so it only exists in the user's checkout on the Mac; if it's missing, ask the user. qBittorrent categories (`movies`, `tv`, ...) have save paths pointing at Jellyfin library folders, so the category chosen decides which library a download lands in.

## How it works

```
                        ┌──────────── 1337tui (tui/) ────────────┐
                        │ search → pick results → pick category  │
                        │ → confirm → exit → exec built command  │
                        └───┬───────────────────────────────┬────┘
           1337cli -s q --json / categories --json   1337cli get <urls> -c <cat>
                            ▼                               ▼
┌───────────────────────── 1337cli (main.go, cmd/) ─────────────────────────┐
│ cmd/root.go      search, table/JSON output, -g picks results to send      │
│ cmd/qbit.go      `get`: page URL → magnet → qBittorrent; qBittorrent flags │
│ cmd/categories.go  `categories`: list qBittorrent categories              │
│ cmd/config.go    loads ~/.config/1337cli/config into env before flags      │
└──────────┬──────────────────────────────────────────┬─────────────────────┘
           ▼                                          ▼
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

1. **Search** (`1337cli -s ubuntu -n 45`): `collect()` in `cmd/root.go` calls `scraper.Client.Search(query, page)` for page after page, until it has `--limit` results or gets a short page (fewer than 20 = last page). An empty page in the middle is retried once, because 1337x sometimes serves one. A later page that fails keeps the results already collected and prints a warning.
2. **Fetch**: `scraper.Client.get` either does a plain HTTP GET, sending `--cookie`/`--user-agent` if set, or POSTs `{"cmd":"request.get"}` to FlareSolverr and uses `solution.response`. Cloudflare's 403/503 bodies are passed to the parser instead of being treated as HTTP errors, so the parser can recognise the challenge page.
3. **Parse**: `ParseSearch` reads `table.table-list tbody tr`. Name and link come from `td.coll-1 a[href^="/torrent/"]`, seeders from `coll-2`, leechers from `coll-3`, date from `coll-date`, size from `coll-4` (its own text only, since a hidden `<span>` repeats the seeder count), uploader from `coll-5`. The title "Just a moment..." means `ErrChallenge`, which `explain()` turns into remediation advice.
4. **Send** (`get` or `-g`): `sendToQbit` turns each target into a magnet link (`Magnet()` fetches the detail page and takes the first `a[href^="magnet:"]`; anything that already starts with `magnet:` is used as is). It logs in once, then adds each torrent. One failure doesn't stop the rest; the command exits non-zero with a count of failures.
5. **qBittorrent specifics** (`internal/qbit`):
   - Every request sends `Referer`/`Origin` set to the base URL, because of qBittorrent's CSRF check.
   - Login succeeds with "Ok." (4.x) or `204` (5.x). "Fails." or `401` means failure.
   - `paused` and `stopped` are both sent, because 5.x renamed the field.
   - **`autoTMM=true` is sent whenever a category is set without `--save-path`.** Without it, qBittorrent ignores the category's save path and the download misses the Jellyfin library.
   - `409` means the torrent is already added.
6. **TUI** (`tui/`): Bubble Tea (`charmbracelet/bubbletea` v1). It moves through the screens `search → searching → results → category → confirm`. Searches and the category list run as background `tea.Cmd`s that call `runJSON`, which execs `1337cli` and shows the CLI's stderr text as the error. Each search has a number (`seq`), and results from cancelled or older searches are dropped. Confirming stores the arguments in `model.runArgs` and quits. `main.go` then prints `$ <command>` and runs it with the terminal's stdin/stdout/stderr. If nothing is selected, the row under the cursor is used.

## Layout

| Path | Purpose |
|---|---|
| `main.go` | Entry point for `1337cli`. Prints the error and exits 1 (Cobra's own error printing is turned off). |
| `cmd/root.go` | Root command, the search and connection flags, `collect`, table output, `--json`, `-g`. |
| `cmd/qbit.go` | `get` subcommand, qBittorrent flags (persistent, so they work on every command), `sendToQbit`. |
| `cmd/categories.go` | `categories` subcommand (table or `--json`). |
| `cmd/config.go` | Config file loader. Runs from a package-level `var`, so it fills the env before any `init()` reads it. |
| `internal/scraper/` | 1337x HTTP client and HTML parsing. `testdata/` holds real or realistic pages. |
| `internal/qbit/` | Minimal qBittorrent Web API client. |
| `tui/` | `1337tui` (package `main`): `cli.go` runs `1337cli` and quotes commands, `model.go` has the screens and keys, `main.go` starts the TUI and runs the chosen command. |
| `deploy/` | Shipped to the server: `install.sh`, `config.example`, `docker-compose.yml` (FlareSolverr, bound to 127.0.0.1). |
| `Makefile` | `make` builds both binaries, `make test` runs the tests, `make dist` builds the server package. |
| `.claude/skills/ship/` | The `/ship` skill: package, verify, copy to the server. |

## Configuration

Precedence: **flag > environment variable > config file**. The config file is `KEY=value` lines (`export` and quotes are allowed), at `os.UserConfigDir()/1337cli/config`. That's `~/.config/1337cli/config` on Linux and `~/Library/Application Support/1337cli/config` on macOS. `LEET_CONFIG` overrides the path.

| Flag | Env / config key | Notes |
|---|---|---|
| `-s, --search` | | Extra positional words are appended, so quotes are optional. |
| `-n, --limit` | | 0 = one page. More than 20 fetches more pages. |
| `-p, --page` | | Start page. |
| `--json` | | Machine output for search and `categories`. Can't be combined with `-g`. |
| `-g, --get` | | Result numbers to send, e.g. `1,3`. |
| `-m, --mirror` | `LEET_MIRROR` | Default `https://1337x.to`. |
| `--flaresolverr` | `FLARESOLVERR_URL` | `/v1` is appended if missing. |
| `--cookie`, `--user-agent` | `LEET_COOKIE`, `LEET_USER_AGENT` | Alternative to FlareSolverr. Both must come from the same browser. |
| `--qbit-url/-user/-pass` | `QBIT_URL`, `QBIT_USER`, `QBIT_PASS` | An empty user skips login. |
| `-c, --category` | `QBIT_CATEGORY` | The category's save path decides the Jellyfin library. |
| `--save-path` | `QBIT_SAVE_PATH` | Overrides the category path (autoTMM is then off). |
| `--paused` | | |
| `1337tui --cli`, `-n` | `LEET_CLI` | Where the TUI finds `1337cli`: `LEET_CLI`, then next to itself, then `$PATH`. |

## Making changes

General rules:

- Keep `1337cli` the only thing that talks to 1337x and qBittorrent. New TUI features should go through a CLI command or flag (with `--json` if the TUI needs to read the output), not import `internal/` directly. That keeps the CLI scriptable and the TUI thin.
- Match the existing style: small packages, errors that tell the user what to do next, a comment explaining *why* something non-obvious is done (CSRF headers, autoTMM, retry on empty page).
- Run `gofmt -l .`, `go vet ./...` and `make test` before calling anything done. Behaviour that touches 1337x or qBittorrent also needs a live check (see Testing).

### Add a flag

- Search or connection flags: `init()` in `cmd/root.go`. qBittorrent flags: `cmd/qbit.go` (persistent). Default from `os.Getenv("KEY")` if it should be configurable, and add the key to `deploy/config.example` and to the table above.
- If the TUI should be able to set it, add it to `getArgs` or `searchArgs` in `tui/cli.go`, to the model and the confirm screen, and to `TestFlowBuildsCommand`.

### Add a subcommand

Create `cmd/<name>.go` with a `cobra.Command`, and register it in that file's `init()` with `rootCmd.AddCommand`. The root command uses `cobra.ArbitraryArgs` so that `-s two words` works. Keep that, or unquoted multi-word searches break once subcommands exist.

### Change JSON output (the CLI–TUI contract)

`scraper.Result` and `qbit.Category` are the source of truth. `tui/cli.go` mirrors them in `result` and `category`. Update both in the same change, and update `TestRunJSONWithFakeCLI`. Only add fields; renaming or removing one breaks a TUI built from an older version.

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

Add a `screen` constant, an `updateX` and a `viewX` in `tui/model.go`, and wire them into `Update` and `View`. Put the new key in that screen's help line. Long-running work must be a `tea.Cmd` that returns a message, never a blocking call inside `Update`. Extend the key-driven tests in `tui_test.go` (`send(m, key(...))`).

### Add a binary or a server-side file

Add it to the `dist` loop in the `Makefile` and to `deploy/install.sh`, then check `tar tzf dist/1337cli.tar.gz` (the `/ship` skill checks the exact file list, so update that list in its `SKILL.md` too).

## Testing

```sh
make test          # unit tests: parser fixtures, qbit client vs httptest, TUI flow, TUI↔CLI contract via a fake 1337cli script
```

Live checks, for changes that touch real services:

- **FlareSolverr:** `docker run -d --name flaresolverr -p 8191:8191 ghcr.io/flaresolverr/flaresolverr`, then `FLARESOLVERR_URL=http://localhost:8191 ./1337cli -s ubuntu -n 5`. A page takes about 12 s.
- **qBittorrent:** `docker run -d --name qbit-test -p 127.0.0.1:18080:18080 -e WEBUI_PORT=18080 -e PUID=1000 -e PGID=1000 lscr.io/linuxserver/qbittorrent`. The temporary admin password is in `docker logs qbit-test`. Wait for the WebUI before logging in. Create categories via `/api/v2/torrents/createCategory`, and create their folders inside the container owned by `abc`. Use small, legal torrents (Linux ISOs) and delete them afterwards with `deleteFiles=true`.
- **TUI:** drive it in `tmux` (`tmux new-session -d -x 120 -y 35 ...`, `send-keys`, `capture-pane -p`).
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
