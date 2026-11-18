# Lodestone product requirements

**Review draft · November 17, 2026 · Baseline `8679a1a`**

Lodestone helps a home media server operator find a torrent, choose where it belongs, and send it to qBittorrent from a terminal. The user can work interactively over SSH or use the same operations in a script. Lodestone’s handoff succeeds when qBittorrent accepts the intended items with the requested destination and start mode. qBittorrent manages the transfer. Jellyfin indexes completed media in its configured libraries.

## Who uses it and what they need

| User | Task | Observable outcome |
| --- | --- | --- |
| Media server operator | Search for a release, compare results, select several, and review the destination before sending. | The confirmed items appear in qBittorrent under the chosen category. Leaving the confirmation screen without submitting queues nothing. |
| CLI or script user | Search without an interactive session, then submit selected page URLs or existing magnets. | A script can parse search output, check the submission exit status, and identify failed items. |
| Server maintainer | Configure the tools once and override a setting for one invocation. | A routine run uses saved defaults. An explicit flag takes effect for that run. |

## Target release scope

The CLI and TUI are independent applications built on the same Go library (`internal/core`). An operator can search, review destinations, and submit items with only the TUI installed. Scripts can continue using the CLI and its JSON output. This scope is implemented; see the status table in the software requirements.

| Capability | Product requirement | Specification |
| --- | --- | --- |
| Discovery | Show enough information to distinguish similarly named releases, including size, upload date, uploader, seeders, and leechers. Support bounded searches and a machine-readable result list. | LS-01 through LS-03 |
| Submission | Accept one or more selections, report which were submitted, and continue if one item in a batch fails. | LS-04, LS-05 |
| Destination and start mode | Use the chosen qBittorrent category’s server-side path or an explicit CLI path override. Let users add items paused. | LS-06, LS-07 |
| Interactive review | Support keyboard selection and preview the items, destination, and start mode before dispatch. A cancelled search must never replace a newer result list. | LS-08, LS-09 |
| One-step CLI selection | A CLI user can search, choose, and send in one command, by result number or range, by most seeders, or at a prompt they ask for, without copying URLs. Scripts are unaffected. Downloads always go through qBittorrent; the tools never download content or show download progress. | LS-14 |
| Operation and compatibility | Support reusable settings and preserve the CLI contract used by scripts. Ship a Linux package with example configuration. | LS-10 through LS-12 |

## Pending feature for review

**Choose a plain server folder in the TUI.** An operator may already have a Jellyfin library directory that has no corresponding qBittorrent category. The deployment example mentions `QBIT_FOLDERS` (marked not yet implemented), but the category listing returns only qBittorrent categories and the TUI submits only category names. The CLI already supports an explicit save path. LS-13 defines the missing selection workflow.

**Open decision.** When a configured folder has the same display name as a category, how should the picker distinguish them? A proposed default is to show the entry type and full server path so the operator can identify either destination before confirming.

## Evidence

[Repository guide at the reviewed commit](https://github.com/jordanjust34/Lodestone/blob/8679a1a/AGENTS.md) · [Deployment configuration](https://github.com/jordanjust34/Lodestone/blob/8679a1a/deploy/config.example) · [Category command](https://github.com/jordanjust34/Lodestone/blob/8679a1a/cmd/categories.go) · [TUI command builder](https://github.com/jordanjust34/Lodestone/blob/8679a1a/tui/cli.go)
