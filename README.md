# Lodestone

Lodestone searches 1337x from your terminal and sends selected torrents to qBittorrent. Use **1337tui** to browse results over SSH or **1337cli** to search and submit from a script. Both Go programs call the same library in `internal/core`. Either runs on its own.

For a Jellyfin setup, give each qBittorrent category a save path that Jellyfin watches. Selecting `movies`, for example, sends the torrent to qBittorrent with that category. qBittorrent handles the transfer and shows its progress. Jellyfin handles library scanning.

## Try it from source

With Git, Make, and the Go toolchain specified in [go.mod](go.mod) installed, run:

```sh
git clone https://github.com/jordanjust34/Lodestone.git
cd Lodestone
make
./1337cli -s ubuntu -n 5
```

Search alone needs access to the source site. To send a result, configure a reachable qBittorrent Web UI. To browse interactively, run `./1337tui`.

## Configure a destination

Both programs read the same `KEY=value` file. On Linux the default is `~/.config/1337cli/config` (or under `$XDG_CONFIG_HOME` when set). On macOS it is `~/Library/Application Support/1337cli/config`. Set `LEET_CONFIG` to use another filename or path.

```ini
QBIT_URL=http://localhost:8080
QBIT_USER=admin
QBIT_PASS=
QBIT_CATEGORY=movies
```

Set the password locally and choose a category that exists on your server. Use the Web UI address reachable **from the machine running Lodestone**. An empty username skips the login request. Keep personal credentials out of version control.

| Setting | Purpose |
| --- | --- |
| `QBIT_CATEGORY` | Default qBittorrent category for submissions. |
| `QBIT_SAVE_PATH` | Explicit directory as qBittorrent sees it. Leave empty to follow the category’s save path. |
| `LEET_MIRROR` | Base URL for the supported 1337x source. |
| `FLARESOLVERR_URL` | Endpoint for an existing FlareSolverr service, if configured. |

CLI flags override environment variables, which override file settings. Run `./1337cli categories --json` to check the available categories and server paths. See [config.example](deploy/config.example) for the full template. Its commented `QBIT_FOLDERS` entry describes a planned folder picker.

## Choose and submit

**Interactive terminal.** Start `./1337tui -n 50` to request up to 50 results. Use the arrow keys or `j/k` to move and Space to select rows. With no explicit selection, Enter uses the highlighted row. Pick a category, review the selection, and press Enter to send or `q` to quit at confirmation. Press `p` on the confirmation screen to toggle paused submission.

**Command line.** These examples assume the binaries are on your PATH and the example category exists.

```sh
1337cli -s ubuntu -n 10 --json          # inspect results in a script
1337cli -s ubuntu -i                    # choose results at a terminal prompt
1337cli -s ubuntu -g 1,3-5 -c linux     # submit numbered results
1337cli -s ubuntu --best -c linux       # submit the most-seeded result
1337cli get - --paused < targets.txt    # read page URLs or magnets from stdin
```

Search JSON contains each result’s name, URL, seeders, leechers, date, size, and uploader. An empty search returns `[]`. Choose one of `--json`, `-g`, `--best`, or `-i` per search. Batch submission continues after individual failures and exits nonzero if any item failed.

## Development

Run `make test` for the Go tests and `make` to build both binaries. The CLI lives in [cmd/](cmd/), the interactive interface in [tui/](tui/), and shared configuration, search, and submission in [internal/core/](internal/core/). The [repository guide](AGENTS.md) explains the request flow. The [requirements](docs/reqs/) describe expected behavior.
