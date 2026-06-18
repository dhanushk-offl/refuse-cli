<div align="center">

# refuse-cli

**Wraps 18 package managers — `npm`, `pnpm`, `yarn`, `bun`, `npx`, `pip`, `pip3`, `uv`, `poetry`, `pipenv`, `pdm`, `pipx`, `cargo`, `gem`, `bundle`, `go`, `composer`, `dotnet` — as PATH shims, plus a Claude Code pre-tool-use hook. Refuses to install packages with known CVEs.**

[![CI](https://github.com/RefuseHQ/refuse-cli/actions/workflows/ci.yaml/badge.svg)](https://github.com/RefuseHQ/refuse-cli/actions/workflows/ci.yaml)
[![Lint](https://github.com/RefuseHQ/refuse-cli/actions/workflows/lint.yaml/badge.svg)](https://github.com/RefuseHQ/refuse-cli/actions/workflows/lint.yaml)
[![CodeQL](https://github.com/RefuseHQ/refuse-cli/actions/workflows/codeql.yml/badge.svg)](https://github.com/RefuseHQ/refuse-cli/actions/workflows/codeql.yml)
[![Release](https://img.shields.io/github/v/release/RefuseHQ/refuse-cli?display_name=tag&sort=semver)](https://github.com/RefuseHQ/refuse-cli/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/RefuseHQ/refuse-cli.svg)](https://pkg.go.dev/github.com/RefuseHQ/refuse-cli)
[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

</div>

```sh
$ npm install lodash@4.17.10
refuse: blocked — CVE-2019-10744 (high)
        Prototype pollution in lodash <= 4.17.11
        suggested safe version: 4.17.21
```

`refuse` sits in front of your package managers. Every `install` call is vetted against a [`refuse`](https://github.com/RefuseHQ/refuse) server (self-hosted) or [refuse.dev](https://refuse.dev) (hosted) before the real binary runs. If the package has a known advisory above your severity threshold, the install is blocked with the CVE and a suggested safe version.

Works:

- On a developer's laptop.
- In a CI runner.
- Inside a Docker build stage.
- As a [Claude Code](https://www.anthropic.com/claude-code) PreToolUse hook — same gate for autonomous agent installs.

---

## Install

**Homebrew** (macOS):

```sh
brew install refusehq/tap/refuse
```

**Direct binary on macOS / Linux**:

```sh
curl -sSL https://raw.githubusercontent.com/RefuseHQ/refuse-cli/main/scripts/install.sh | sh
```

**Direct binary on Windows** (PowerShell):

```powershell
irm https://raw.githubusercontent.com/RefuseHQ/refuse-cli/main/scripts/install.ps1 | iex
```

**From source**:

```sh
go install github.com/RefuseHQ/refuse-cli/cmd/refuse@latest
```

**Platforms.** Pre-built binaries are published for:

| OS | Architectures |
| --- | --- |
| macOS | x86_64, arm64 |
| Linux | x86_64, arm64, i386, armv6, armv7 |
| Windows | x86_64, arm64, i386 |

Other platforms can `go install` from source.

## Quickstart

```sh
refuse init                       # interactive: server URL + API key
refuse install                    # writes shims to ~/.refuse/bin + updates PATH
refuse hook install claude-code   # PreToolUse hook in ~/.claude/settings.json
refuse python-hook install        # closes the `python -m pip` bypass (per Python env)
```

Then run anything you'd normally run:

```sh
npm install express
pip install requests
cargo add tokio
```

If the install is clean, it goes through. If it isn't, refuse blocks it and tells you why.

## Server

The CLI is the gate; the server is the brain. Two ways to bring up a backend — pick either:

**Hosted** ([refuse.dev](https://refuse.dev))

```sh
refuse config set server_url https://mcp.refuse.dev
refuse config set api_key rfs_...
```

**Self-hosted** ([RefuseHQ/refuse](https://github.com/RefuseHQ/refuse) — Apache-2.0, single Docker container, no signup)

```sh
docker run -d --name refuse -p 8080:8080 \
  -v refuse-data:/data \
  ghcr.io/refusehq/refuse:latest
refuse config set server_url http://localhost:8080
```

The server ingests OSV, CISA KEV, FIRST EPSS, GHSA, deps.dev, and Wolfi; first-boot bootstrap takes ~3 min, then deltas every 5 min. `GET /readyz` flips from 503 to 200 when the seed completes. Both editions speak the same `/api/v1/check/*` API, so switching between hosted and self-host is a one-line config change.

## Supported package managers

| Manager | Ecosystem | Install verbs | Lockfile parsing |
| --- | --- | --- | --- |
| `npm` | npm | `install`, `i`, `add` | `package-lock.json` |
| `pnpm` | npm | `install`, `add` | `pnpm-lock.yaml` |
| `yarn` (classic + Berry) | npm | `add`, `install` | `yarn.lock` |
| `bun` | npm | `install`, `add` | `bun.lockb` / `bun.lock` |
| `pip` / `pip3` | PyPI | `install`, `install -r` | `requirements.txt` |
| `cargo` | crates.io | `add`, `install` | `Cargo.lock` |
| `gem` | RubyGems | `install` | `Gemfile.lock` |
| `go` | Go modules | `get`, `install` | `go.sum` |

## Supported agent hooks

| Agent | Status |
| --- | --- |
| Claude Code | ✓ supported |
| Codex CLI | ✓ supported |
| Cursor | tracked |
| Continue | tracked |
| Aider | tracked |
| Cline | tracked |

PRs welcome — see [`internal/hook/claudecode.go`](./internal/hook/claudecode.go) as the reference.

## Commands

| Command | What it does |
| --- | --- |
| `refuse init` | First-time setup wizard |
| `refuse install` | Install shims for the supported package managers |
| `refuse uninstall` | Remove shims + revert shell-rc edits |
| `refuse hook install <agent>` | Write a pre-tool-use hook for `<agent>` |
| `refuse hook remove <agent>` | Remove that hook |
| `refuse hook list` | Show all installed hooks |
| `refuse check <eco> <pkg>[@<ver>]` | One-off check |
| `refuse check-lockfile <path>` | Scan an entire lockfile |
| `refuse audit [path]` | One-shot scan of every lockfile / Dockerfile / GH-Actions workflow under a directory |
| `refuse gate` | The decision engine — shims + hooks call this on stdin |
| `refuse pip-gate` | Variant of `refuse gate` for pip's argument grammar; invoked by the pip shim. |
| `refuse config show \| set \| get` | Manage `~/.refuse/config.yaml` |
| `refuse status` | Diagnose install state |
| `refuse doctor` | Verify PATH / hooks / server reachability |

```sh
refuse audit                  # walk the repo, scan lockfiles + Dockerfiles + workflows
refuse audit --json > out.json
```

## Configuration

`refuse config set <key> <value>`, or edit `~/.refuse/config.yaml` directly:

```yaml
server_url: http://localhost:8080        # or https://mcp.refuse.dev
api_key: rfs_...                         # optional, required for hosted
policy:
  severity_threshold: high               # low | medium | high | critical
  fail_closed: false                     # true = block if server unreachable (default false → fail open with stderr warning)
  allow_yanked: false                    # allow yanked versions when no advisory matches
  allow_prerelease: false                # allow prerelease versions
  override_env: REFUSE_ALLOW_VULNERABLE  # env var that forces a block to pass through
```

Environment variables override the file (useful in CI):

- `REFUSE_SERVER_URL`
- `REFUSE_API_KEY`
- `REFUSE_POLICY` — sets `severity_threshold`
- `REFUSE_FAIL_CLOSED` — `1`/`true` to enable
- `REFUSE_ALLOW_VULNERABLE` — `1`/`true` to bypass a single install
- `REFUSE_TIMEOUT_MS` — HTTP timeout in milliseconds (default `8000`)
- `REFUSE_NO_GATE` — `1` to skip the gate entirely for the next call (debug)

### Bypassing the gate for one command

Append `--no-refuse` to any wrapped command to skip the gate just this once.
refuse strips the flag before running the real manager:

```sh
npm install lodash@4.17.10 --no-refuse   # runs the real npm, ungated
pip install pyyaml==5.3 --no-refuse
```

Equivalent to a one-shot `REFUSE_NO_GATE=1`, but inline. This only works
through the PATH shim (interactive use) — the agent PreToolUse hook
ignores it, so an autonomous agent can't bypass its own gate.

## Enforcement layer — pre-commit + CI

The PATH shim is a convenient first line, but anything that bypasses it
(`python -m pip`, `uv`, absolute-path calls, `conda install`, a Dockerfile
layer that doesn't inherit the shim PATH) won't be gated. The **deterministic**
gate is `refuse check-lockfile` — it inspects the resolved manifest no
matter how a dep got there.

Wire it into both ends of the pipeline:

> **`refuse proxy` (preview)** — local HTTPS forward proxy that gates registry traffic for ecosystems the shim can't reach (Buildkit, isolated CI agents). Status: experimental. See [docs/design/registry-proxy.md](docs/design/registry-proxy.md).

### Pre-commit (local)

Block a vulnerable lockfile from being committed in the first place. In
your project's `.pre-commit-config.yaml`:

```yaml
repos:
  - repo: https://github.com/RefuseHQ/refuse-cli
    rev: <version>       # pin a refuse release — see https://github.com/RefuseHQ/refuse-cli/releases
    hooks:
      - id: refuse-check
      # Optional, heavier — only on push:
      - id: refuse-check-installed-pip
```

`refuse-check` scans every common lockfile that changed in the commit
(`package-lock.json`, `yarn.lock`, `pnpm-lock.yaml`, `requirements.txt`,
`Pipfile.lock`, `poetry.lock`, `pdm.lock`, `uv.lock`, `Cargo.lock`,
`Gemfile.lock`, `go.sum`, `composer.lock`, `pom.xml`, `*.csproj`,
`mix.lock`, `pubspec.lock`) and aborts the commit on any vulnerable hit.

`refuse-check-installed-pip` runs on `pre-push` only and pipes
`pip freeze` through `refuse check-lockfile`, catching deps that arrived
via `python -m pip`, `uv pip`, or `conda install`.

### GitHub Actions (CI)

Use the bundled composite action:

```yaml
# .github/workflows/ci.yaml
jobs:
  refuse:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: RefuseHQ/refuse-cli@v1
        with:
          api-key: ${{ secrets.REFUSE_API_KEY }}
          # lockfiles: |       # optional override — auto-discovers if blank
          #   package-lock.json
          #   requirements.txt
```

Auto-discovers every supported lockfile in the workspace and fails the
build on a vulnerable hit. See [`action.yml`](./action.yml).

| Input | Default | Required | Description |
| --- | --- | --- | --- |
| `server-url` | `https://mcp.refuse.dev` | no | refuse server URL (override for self-host). |
| `api-key` | none | no | refuse API key (`rfs_...`). Required for hosted; optional for localhost self-host. |
| `lockfiles` | auto-discover | no | Newline-separated list of lockfile paths. Auto-discovers common formats if empty. |
| `version` | `latest` | no | refuse-cli version to install. |
| `severity` | none | no | Severity floor (`low` \| `medium` \| `high` \| `critical`). |
| `fail-on-error` | `true` | no | Fail the workflow on server/network errors. `false` falls through. |

### One-off / arbitrary scan

```sh
# Scan an explicit lockfile
refuse check-lockfile package-lock.json

# Scan currently-installed pip set (catches `python -m pip` / conda)
pip freeze | refuse check-lockfile --filename=requirements.txt /dev/stdin
```

## Contributing

PRs welcome — particularly for new package managers, agent hooks, and platforms. See [CONTRIBUTING.md](./CONTRIBUTING.md).

## Security

Security policy: [SECURITY.md](./SECURITY.md). Report privately via [hello@refuse.dev](mailto:hello@refuse.dev).

## License

[Apache License 2.0](./LICENSE) © RefuseHQ.
