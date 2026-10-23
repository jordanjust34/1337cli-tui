---
name: ship
description: Package 1337cli and 1337tui into the release tarball for the user's Linux server (mini-PC running qBittorrent/Jellyfin), verify it, and optionally scp it over. Use when the user asks to compress, package, ship, deploy or "send the project to the server".
argument-hint: "[user@server-ip]"
---

# Ship 1337cli to the server

Target: `$ARGUMENTS` (empty means build and verify only; don't copy).

"Compress the project" means the **release tarball** built by `make dist`, never a source archive or a hand-rolled `tar`/`zip`.

## 1. Pre-flight

Run from the repo root:

```sh
git status --short
```

If there are uncommitted changes, mention it in the final report (the package is built from the working tree, so it includes them). Don't stop or commit on your own.

## 2. Build

```sh
make dist
```

This runs `go test ./...` first and aborts on failure, then cross-compiles both binaries (`linux/amd64`, `linux/arm64`, `linux/arm` v7, `windows/amd64`, `CGO_ENABLED=0`), copies `deploy/` files, and writes `dist/1337cli.tar.gz` with Apple xattrs stripped.

If it fails, stop and show the user the failing test or compile error. Don't ship a stale `dist/1337cli.tar.gz` left over from an earlier build.

## 3. Verify

```sh
tar tzf dist/1337cli.tar.gz
ls -lh dist/1337cli.tar.gz
shasum -a 256 dist/1337cli.tar.gz
```

The listing must be exactly:

```
1337cli/
1337cli/install.sh
1337cli/config.example
1337cli/docker-compose.yml
1337cli/bin/
1337cli/bin/1337cli-{linux-amd64,linux-arm64,linux-arm,windows-amd64.exe}
1337cli/bin/1337tui-{linux-amd64,linux-arm64,linux-arm,windows-amd64.exe}
```

Fail loudly if anything is missing, or if anything else is in there. In particular, a file named `config` or containing `QBIT_PASS=` with a value means secrets leaked in. Only `config.example` may ship.

If a new binary or server-side file was added to the project, it belongs in the `dist` target of the `Makefile` and in `deploy/install.sh`; fix those rather than patching the tarball.

## 4. Copy (only if a target was given)

If `$ARGUMENTS` is empty, skip this step and give the user the command instead.

If a target was given, it must look like `user@host`. Copy with:

```sh
scp dist/1337cli.tar.gz "$ARGUMENTS":~
```

- The trailing `:~` is required. Without a colon, `scp` writes a local file named `user@host` (this has happened before in this repo).
- If the user passed `user@host:8080` or similar, strip the port: that's qBittorrent's web port, and `scp` uses SSH.
- If `scp` fails with "connection refused", SSH isn't enabled on the server (`sudo apt install openssh-server`). If it fails with "permission denied", the login or key is wrong. Report it; don't retry repeatedly.

Then confirm the copy arrived intact:

```sh
ssh "$ARGUMENTS" 'sha256sum ~/1337cli.tar.gz'
```

It must match the local hash. Don't run `install.sh` over SSH yourself, because it can prompt for a sudo password. Leave that to the user.

## 5. Report

Tell the user:

- the tarball path, size and SHA-256
- whether it was copied, and whether the remote hash matched
- any uncommitted changes that went into the build
- the commands to run on the server:

```sh
scp dist/1337cli.tar.gz <user>@<server-ip>:~      # only if not copied already
tar xzf 1337cli.tar.gz && cd 1337cli && ./install.sh
```

`install.sh` is safe to re-run for upgrades: it replaces both binaries, keeps the existing `~/.config/1337cli/config`, and (re)starts FlareSolverr.
