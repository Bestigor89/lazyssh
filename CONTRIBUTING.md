# Contributing to sshmanager

Thank you for your interest in contributing! sshmanager is a keyboard-driven SSH/SFTP
manager written in Go. Contributions of all sizes are welcome — from a one-line typo fix
to a whole new feature.

---

## Table of contents

1. [Before you start](#before-you-start)
2. [Development setup](#development-setup)
3. [Project layout](#project-layout)
4. [Making changes](#making-changes)
5. [Code style](#code-style)
6. [Running tests](#running-tests)
7. [Submitting a pull request](#submitting-a-pull-request)
8. [Good first issues](#good-first-issues)
9. [Licensing of contributions](#licensing-of-contributions)

---

## Before you start

- Check the [issue tracker](../../issues) to see if your idea or bug is already being
  discussed. Opening an issue before a large PR saves everyone time.
- For small fixes (typos, documentation, obvious bugs) feel free to open a PR directly.

---

## Development setup

**Requirements**

| Tool | Version |
|------|---------|
| Go   | 1.21+   |
| make | any     |
| git  | any     |

Runtime (not for building, but needed to test all features):
- `ssh` binary on `$PATH` (interactive terminal feature)
- `$EDITOR` set (file edit feature)

**Clone and build**

```bash
git clone https://github.com/igorivitskyy/sshmanager.git
cd sshmanager
make build       # produces ./sshmanager
make run         # build + launch immediately
```

Useful Makefile targets:

```
make build    Compile the binary
make install  Install to $GOPATH/bin
make run      Build and run
make test     Run the test suite
make fmt      Format all Go files with gofmt
make vet      Run go vet
make tidy     Tidy go.mod / go.sum
make clean    Remove build artifacts
```

---

## Project layout

```
cmd/sshmanager/     Entry point: CLI flags, version, TUI bootstrap
internal/
  model/            Host struct, Store, ID generation
  config/           YAML persistence, AES-256-GCM password encryption
  ssh/              SFTP client (upload/download/browse), SSH terminal launcher
  ui/               tview TUI: host list, host form, file browser, modals
```

**Where to make your change:**

| You want to… | Look in |
|---|---|
| Change the host data model | `internal/model/host.go` |
| Change how config is read/written | `internal/config/config.go` |
| Change encryption | `internal/config/crypto.go` |
| Add a new SFTP operation | `internal/ssh/sftp.go` |
| Change SSH terminal launch | `internal/ssh/terminal.go` |
| Change the host list screen | `internal/ui/hostlist.go` |
| Change the add/edit form | `internal/ui/hostform.go` |
| Change the file browser | `internal/ui/filebrowser.go` |
| Add modals or change navigation | `internal/ui/app.go` |

---

## Making changes

1. Fork the repo and create a feature branch: `git checkout -b feat/my-feature`.
2. Make your changes. Keep commits focused — one logical change per commit.
3. Add or update tests for any changed behaviour (see [Running tests](#running-tests)).
4. Run `make fmt && make vet` and confirm both pass before pushing.

**Commit message style:**

```
type: short present-tense summary (≤72 chars)

Optional longer explanation. Wrap at 72 chars. Explain *why*,
not just *what*.
```

Types: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`.

---

## Code style

- Standard Go formatting via `gofmt` / `make fmt` — no style debates.
- `make vet` must pass with zero output.
- Avoid adding new external dependencies without a discussion in an issue first.
- TUI code uses [rivo/tview](https://github.com/rivo/tview) patterns — keep new UI code
  consistent with the existing widgets and key-binding conventions.

---

## Running tests

```bash
make test          # run the whole suite
go test ./internal/config/...   # or a specific package
go test -race ./...             # with race detector (recommended before submitting)
```

Current test coverage focuses on the `config` (crypto) and `model` packages. If you add a
feature in `ssh` or `ui`, even a basic smoke test is appreciated.

---

## Submitting a pull request

1. Push your branch and open a PR against `main`.
2. Fill in the PR description: what changed, why, how to test.
3. The project maintainer will review and may request changes.
4. Once approved, the PR is merged with a merge commit to keep history readable.

Please be patient — this is a solo-maintained project.

---

## Good first issues

Looking for a place to start? Here are concrete ideas, roughly ordered by complexity:

| Difficulty | Idea |
|---|---|
| 🟢 Easy | Add `--help` flag to the CLI |
| 🟢 Easy | Tests for `internal/model` and `internal/config` edge cases |
| 🟢 Easy | GitHub Actions CI workflow (`go test`, `go vet`, `go build` on push) |
| 🟡 Medium | Homebrew formula / release binaries via GoReleaser |
| 🟡 Medium | Support passphrase-protected private keys (prompt at connect time) |
| 🟡 Medium | Configurable color themes (read from `hosts.yaml` or a separate file) |
| 🟡 Medium | Mouse support (tview has `EnableMouse` — needs UX design) |
| 🔴 Hard | Port forwarding / SSH tunnel management |
| 🔴 Hard | Parallel transfers with a transfer queue and status panel |
| 🔴 Hard | Resume interrupted file transfers |
| 🔴 Hard | Run a command across all hosts in a folder/tag group |
| 🔴 Hard | Windows support (currently untested; mostly needs terminal handling fixes) |

---

## Licensing of contributions

sshmanager uses a **dual-license model**: non-commercial use is covered by
[PolyForm Noncommercial 1.0.0](LICENSE); commercial use requires a separate
[commercial license](COMMERCIAL-LICENSE.md).

By submitting a pull request you agree that your contribution is made under the same
PolyForm Noncommercial 1.0.0 terms **and** that the project maintainer (Igor Ivitskyy)
may include your contribution in commercially-licensed versions of the software. This is
the standard arrangement for dual-licensed projects (sometimes called an
"inbound = outbound + commercial relicensing grant"). If this is a problem for you, please
raise it before contributing.

You retain copyright in your own contributions.
