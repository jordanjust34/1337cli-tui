# Plan: choose and send from one CLI invocation

**Status:** planned, not started. Builds on the shared-library refactor (LS-11) on branch `issue/shared-lib`.

## Goal

Today a CLI user runs `1337cli -s query`, reads the table, and copy-pastes a page URL into `1337cli get <url>`. The goal is to search, choose, and send in one invocation. The TUI already works this way.

## Decisions (agreed with the user)

1. **All downloads go through qBittorrent.** The programs only resolve magnet links and hand them to qBittorrent. They never download torrent content themselves (no embedded BitTorrent engine).
2. **The interactive prompt is opt-in**, via a flag (`-i, --interactive`). Without the flag the CLI behaves as it does today. There is no config key to make the prompt the default.
3. **No download progress display** in either front end. Users watch progress in qBittorrent's WebUI.

## Changes

### `internal/core` (prints nothing)

```go
// Best returns the n results with the most seeders (ties keep source order); n <= 0 means 1.
func Best(results []Result, n int) []Result

// Ping logs in and returns qBittorrent's version, so front ends can fail before
// a ~15 s search when the server is unreachable or the login is wrong.
func (s *Service) Ping() (version string, err error) // wraps qbit.Client.Version
```

`Submit` keeps its signature (the TUI `backend` interface depends on it).

### CLI (`cmd/`)

Existing flags, JSON fields and exit codes stay the same.

| Change | Detail |
|---|---|
| `-i, --interactive` | After the table, prompt on stderr: `Send which? (e.g. 1,3 or 2-5, enter to quit):`. If no `-c`/`QBIT_CATEGORY` is set, a second prompt lists categories by number (`enter` = server default). Invalid input asks again; empty input or EOF exits 0 and sends nothing. Errors if stdin isn't a terminal, and can't be combined with `--json`, `-g` or `--best`. |
| `-g` ranges | Switch from `IntSliceVarP` to `StringSliceVarP`, parsed by `parseSelection` in `cmd/`. `-g 1,3` and `-g 1 -g 3` still work; `-g 2-4` and `-g all` are new. Keep the out-of-range error text. |
| `--best` | Send the top-seeded result without prompting: `1337cli -s "ubuntu 24.04" --best -c linux`. Not with `--json`, `-g` or `-i`. |
| `get -` | Read targets from stdin, one per line: `1337cli -s x --json \| jq -r '.[0].url' \| 1337cli get -`. |
| Early qBittorrent check | When `-g`, `--best` or `-i` will submit, call `Ping()` before searching. |

`RunE` in `cmd/root.go` becomes: search → `chooseTargets(results)` (decides between `-g`, `--best`, `-i`, or nothing) → `sendToQbit`. `sendToQbit` stays the only place that submits and sets the exit status.

### TUI (`tui/`), optional

- `b` on the results screen selects the top-seeded result (`core.Best`). Add it to the help line.
- Call `Ping` at startup alongside `loadCategories`, and show "qBittorrent unreachable" on the search screen early.

## Subtasks and dependencies

| # | Task | Files | Depends on |
|---|---|---|---|
| A0 | Commit the shared-library refactor | current changes | — |
| A1 | `core.Best`, `Service.Ping` + tests | `internal/core/core.go`, `core_test.go` | A0 |
| A2 | `parseSelection` + `-g` as strings | `cmd/select.go`, `cmd/select_test.go`, `cmd/root.go` | A0 |
| A3 | `get -` | `cmd/qbit.go` | A0 |
| A4 | TUI `b` key, startup ping (optional) | `tui/model.go`, `tui/tui_test.go` | A1 |
| A5 | `-i` prompt: `pickTargets(in io.Reader, out io.Writer, results, cats)` | `cmd/prompt.go`, `cmd/prompt_test.go`, `cmd/root.go` | A2 |
| A6 | `--best` and the `chooseTargets` refactor | `cmd/root.go` | A1, A2 |
| A7 | Docs | see below | A3, A5, A6 |
| A8 | Live check: FlareSolverr, throwaway qBittorrent container, tmux for `-i` | — | all |

A1, A2 and A3 can run in parallel. A4, A5 and A6 can run in parallel once their dependencies are done.

## Tests

- `cmd/select_test.go`: `1,3`, `2-4`, `all`, duplicates, `0`, `n+1`, `3-1`, garbage.
- `cmd/prompt_test.go`: drive `pickTargets` with a `strings.Reader` (`"9\n1,3\n2\n"`); assert the re-prompt, targets and options; EOF sends nothing.
- First end-to-end test in `cmd/`: fake 1337x (`httptest`, reusing the `fake1337x` pattern from `internal/core/core_test.go`) and a recording fake qBittorrent; run `rootCmd` with `-g 1-2` and assert two adds. Add a `resetFlags()` helper, since `cmd/` uses package-level flag variables.
- `core_test.go`: `TestBest` (order, ties, n larger than the list), `TestPing`.
- `tui_test.go`: `b` key, if A4 is done.

## Docs to update (A7)

- `AGENTS.md`: CLI box in the diagram, request flow step 4, flags table (`-i`, `-g` ranges, `--best`, `get -`).
- `README.md`: CLI usage section.
- `docs/reqs/SOFTWARE_REQUIREMENTS.md`: extend LS-04 with ranges and `--best`; add LS-14 "in-invocation selection" (prompt only with `-i`, scripts unaffected, EOF sends nothing); update the status table.

## Packaging

No new binaries, files or dependencies (the terminal check uses the standard library). The `Makefile`, `install.sh` and the `/ship` file list don't change.

## Risks

- Changing `-g` from ints to strings changes its help-text type and makes non-numeric errors come from our parser instead of pflag's.
- The prompt goes to stderr and the table to stdout, so `1337cli -s x -i > out.txt` still captures only the table.
- Package-level flag variables in `cmd/` can make tests order-dependent; the reset helper addresses this.

## Out of scope

- An embedded BitTorrent engine or any direct download of torrent content.
- `QBIT_FOLDERS` / plain server folders (separate requirement, LS-13).
