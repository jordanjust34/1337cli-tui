# 1337cli / 1337tui

Search 1337x from a terminal and send torrents to qBittorrent, so they land in the right Jellyfin library.

- **`1337tui`**: interactive. Search, pick results, pick a category, confirm, and it sends them.
- **`1337cli`**: scriptable. Table or JSON search output, and `get` to send page URLs or magnets.

The two are independent programs built on the same Go library (`internal/core`). Either one works without the other installed.

The qBittorrent **category** decides where a download goes: give each category a save path inside a Jellyfin library (e.g. `movies → /media/movies`, `tv → /media/tv`).

## Install (Linux server)

Build the release package on a development machine and copy it over:

```sh
make dist                                  # runs tests, builds dist/1337cli.tar.gz
scp dist/1337cli.tar.gz user@server:~
```

On the server:

```sh
tar xzf 1337cli.tar.gz && cd 1337cli && ./install.sh
```

`install.sh` installs both binaries (to `/usr/local/bin`, or `~/.local/bin` without sudo), creates `~/.config/1337cli/config` from `config.example` if it doesn't exist, and starts FlareSolverr with Docker. It's safe to re-run for upgrades; it keeps your config.

Then edit `~/.config/1337cli/config` and set at least `QBIT_URL`, `QBIT_USER` and `QBIT_PASS`.

## Configuration

Both programs read the same settings. Precedence: **command-line flag (CLI only) > environment variable > config file**.

The config file is `KEY=value` lines at `~/.config/1337cli/config` (Linux) or `~/Library/Application Support/1337cli/config` (macOS). Set `LEET_CONFIG` to use another path.

| Key | CLI flag | Meaning |
|---|---|---|
| `QBIT_URL` | `--qbit-url` | qBittorrent Web UI, e.g. `http://localhost:8080` |
| `QBIT_USER`, `QBIT_PASS` | `--qbit-user`, `--qbit-pass` | Login. Empty user skips login (for "bypass auth on localhost"). |
| `QBIT_CATEGORY` | `-c, --category` | Default category. In the TUI, picking "none" uses it. |
| `QBIT_SAVE_PATH` | `--save-path` | Fixed folder on the server; overrides the category's path. Usually empty. |
| `FLARESOLVERR_URL` | `--flaresolverr` | FlareSolverr endpoint for getting past Cloudflare, e.g. `http://localhost:8191` |
| `LEET_COOKIE`, `LEET_USER_AGENT` | `--cookie`, `--user-agent` | Alternative to FlareSolverr: a `cf_clearance` cookie and the User-Agent of the browser it came from. |
| `LEET_MIRROR` | `-m, --mirror` | 1337x mirror. Default `https://1337x.to`. |

`QBIT_FOLDERS` appears in `config.example` but isn't implemented yet.

## Using the TUI

```sh
1337tui          # 20 results per search
1337tui -n 50    # more results per search
```

| Screen | Keys |
|---|---|
| Search | type a query, `enter` search, `esc` back to results, `ctrl+c` quit |
| Results | `↑/↓` or `j/k` move, `space`/`x` select, `a` select all, `enter` continue, `/` new search, `q` quit |
| Category | `↑/↓` move, `enter` continue, `esc` back |
| Confirm | `enter`/`y` send, `p` toggle paused, `esc` back, `q` quit without sending |
| Done | `enter` new search, `esc` back to results, `q` quit |

If nothing is selected, the highlighted row is sent. Nothing is sent until you confirm. The TUI exits with status 1 if any torrent failed to send.

## Using the CLI

```sh
1337cli -s ubuntu 24.04              # search (quotes optional), one page
1337cli -s ubuntu -n 50              # fetch pages until 50 results
1337cli -s ubuntu -p 2               # start at page 2
1337cli -s ubuntu --json             # JSON array for scripts
1337cli -s ubuntu -g 1,3 -c linux    # search, then send results 1 and 3

1337cli get <torrent-page-url|magnet>... [-c movies] [--save-path /dir] [--paused]
1337cli categories [--json]          # list categories and their save paths
```

For scripts:

- Search JSON is an array of objects with `name`, `url`, `seeders`, `leechers`, `date`, `size`, `uploader`. No results is `[]`.
- `categories --json` is an array of `{"name", "savePath"}`.
- `get` prints `✓ sent to qBittorrent: <name>` per success on stdout and `✗ <target>: <error>` per failure on stderr. It keeps going after a failure and exits 1 with `N of M torrent(s) failed`.
- Any error exits 1 with the message on stderr.

## Troubleshooting

- **"blocked by Cloudflare challenge"**: set `FLARESOLVERR_URL` (and check `docker ps` shows FlareSolverr running), or use `LEET_COOKIE` + `LEET_USER_AGENT`. If FlareSolverr itself starts failing, `docker compose pull && docker compose up -d` in the install folder.
- **qBittorrent `401` on every request**: the port in `QBIT_URL` must match qBittorrent's own WebUI port. A remapped Docker port fails its Host-header check.
- **Download lands in the wrong folder**: check the category's save path in qBittorrent (right-click the category > Edit), and that `QBIT_SAVE_PATH` is empty.
- **Torrent shows `error` in qBittorrent**: qBittorrent can't write to the category's folder; check its owner and permissions.

## Development

```sh
make             # build ./1337cli and ./1337tui
make test        # unit tests
make dist        # release package
```

See [`AGENTS.md`](AGENTS.md) for architecture and how to change things, and [`docs/reqs/`](docs/reqs/) for requirements.
