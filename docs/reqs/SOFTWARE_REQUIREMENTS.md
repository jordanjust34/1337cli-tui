# Lodestone software requirements and acceptance criteria

**Review draft · November 17, 2026 · Baseline `8679a1a`**

At the reviewed commit, the TUI executes the CLI for searches, category lookups, and submissions. LS-11 defines the planned migration to two independent applications using a shared Go library. LS-13 describes a separate pending destination feature. The acceptance checks below describe the intended behavior and have not been run.

## Search and submission

| ID | Requirement | Acceptance check |
| --- | --- | --- |
| LS-01 | The CLI shall accept multiword queries, a starting page of at least 1, and a nonnegative result limit. A limit of zero requests one page. | Given two fixture pages of 20 entries each, request 23 and receive exactly 23 in source order. With limit zero, receive only the first page. A negative limit or page zero yields an error. |
| LS-02 | Search shall support a human-readable table and JSON output. Each JSON result shall include name, URL, seeders, leechers, date, size, and uploader. | Parse stdout from a successful JSON search and check all seven fields. An empty search result encodes as an empty array. Combining JSON output with indexed submission yields an error before dispatch. |
| LS-03 | Search shall retain collected results when a later page fails and retry an unexpectedly empty later page once. | Serve 20 rows followed by an empty page and then one row on retry. Expect 21 rows. Repeat with a later-page error and expect 20 rows plus a warning on stderr. A first-page error must reach the caller. |
| LS-04 | The CLI shall accept one-based search result indices, torrent page URLs, and direct magnets. | Select results 1 and 3 and capture the two submitted targets. A nonexistent index prevents dispatch. For direct input, verify a page URL resolves to a magnet and a supplied magnet skips page lookup. |
| LS-05 | A per-item failure shall allow remaining batch items to proceed. An incomplete batch shall return a nonzero exit status and a failure count. | Submit three targets to a test service that rejects only the second. Verify the third was attempted, two successes were reported, and the summary reports one failure out of three. |

## Destination and interactive review

| ID | Requirement | Acceptance check |
| --- | --- | --- |
| LS-06 | The CLI shall list server categories and associated paths. A category-only submission shall use qBittorrent’s category routing. An explicit save path takes priority. | Configure a test category with a path different from the server default. Verify it appears in category JSON as `name` and `savePath`. Submit once with the category and once with an overriding path, then check both destinations in qBittorrent. |
| LS-07 | Users shall be able to request a paused submission from either interface. | Add a test item with `--paused`. Repeat using the TUI pause toggle. Verify the client receives a stopped or paused item in each case. |
| LS-08 | The TUI shall preview selected items, category, and start mode before dispatch. With no explicit selection it shall use the highlighted row. | Select two rows, review the summary, and quit. A recording implementation of the shared service must observe zero submissions. Repeat and confirm to observe one batch containing both URLs. Repeat with only a highlighted row and expect that URL alone. |
| LS-09 | Search and category lookups shall allow the TUI to process input while waiting. Stale search responses shall be ignored. | Delay search A, cancel it, and start search B. Deliver B and then A. The results screen must still show B. |

## Configuration, integration, and delivery

| ID | Requirement | Acceptance check |
| --- | --- | --- |
| LS-10 | For settings available in all three places, explicit flags shall override environment values, which shall override file settings. | Give the qBittorrent URL three distinct test values. Observe the flag endpoint, then remove the flag and observe the environment endpoint. Start a new process without either override and observe the file endpoint. |
| LS-11 | Both binaries shall call the same Go library directly for search, destination lookup, magnet resolution, and submission. The CLI owns flag parsing, JSON, and exit status. The TUI owns selection, confirmation, and rendering. Each binary shall function independently. | Build both apps. In an environment containing only the TUI binary and test services, search, list destinations, and confirm an item. Verify the service receives the chosen item, destination, and start mode. Run the equivalent CLI flow and verify the same submission and the existing CLI JSON field names and types. |
| LS-12 | The distributable shall include the Linux binaries and installation material with example configuration. | Build with `make dist`. Inspect the archive for both binaries, installer, compose file, and example config. Check that personal config files and credentials are absent. Exercise installation in a disposable Linux environment. |

## Pending implementation

**LS-13 — Plain server folder selection.** When an operator configures plain folders, the category listing shall also expose those paths as folder entries. Both interfaces shall submit a folder as an explicit server path and a category as a category name. The CLI expresses these choices with `--save-path` and `--category`.

**Acceptance check.** Configure one category named `movies` and one plain folder at `/downloads/tvshows`. Verify both appear in the picker and the folder’s JSON entry has `folder: true`. Confirm each choice separately and inspect the shared service requests. Expect category `movies` for the first and explicit path `/downloads/tvshows` for the second. With no configured folders, verify the existing category flow still works.

## Verification setup

Use deterministic search fixtures for paging and a recording implementation of the shared library interface for TUI checks. Use a disposable qBittorrent instance with distinct default, category, and override paths to verify actual routing. A Linux distribution torrent added paused is sufficient to inspect destination and start mode.

[Search and selection](https://github.com/jordanjust34/Lodestone/blob/8679a1a/cmd/root.go) · [Submission](https://github.com/jordanjust34/Lodestone/blob/8679a1a/cmd/qbit.go) · [TUI state and controls](https://github.com/jordanjust34/Lodestone/blob/8679a1a/tui/model.go) · [Build and packaging](https://github.com/jordanjust34/Lodestone/blob/8679a1a/Makefile)
