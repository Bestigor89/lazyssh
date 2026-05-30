# sshmanager

**A keyboard-driven SSH connection manager and dual-pane SFTP file browser for your terminal.**

[![Go 1.21+](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go)](https://golang.org)
[![License: PolyForm Noncommercial](https://img.shields.io/badge/License-PolyForm_Noncommercial-blue)](LICENSE)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen)](CONTRIBUTING.md)
[![Built with tview](https://img.shields.io/badge/TUI-tview-orange)](https://github.com/rivo/tview)

Manage dozens of servers from a single terminal window. Organize hosts into nested folders, search instantly, jump into an SSH session or a full SFTP file browser with one keystroke — and rest easy knowing passwords are encrypted at rest with Argon2id + AES-256-GCM.

---

<!-- Drop a screenshot or GIF here once you have one:
     docs/screenshot.png  (1200×700 px recommended)

![sshmanager screenshot](docs/screenshot.png)
-->

---

## Why sshmanager?

Most SSH managers are GUI apps or `.ssh/config` wrappers with no file transfer story.
sshmanager lives entirely in your terminal and gives you:

- **Folders + tags** — group 50 servers so you can find the one you need in two keystrokes.
- **One-key SFTP browser** — navigate, upload, download, edit remote files, all without leaving the app.
- **Encrypted secrets** — stored passwords are never plain text (Argon2id key derivation + AES-256-GCM, one master password per session).
- **Zero friction** — no daemon, no config server, no Docker. One static binary, one YAML file.

---

## Features

### Host management
- Add, edit, delete hosts through a form-based UI
- Group hosts in **nested folders** (`prod/web`, `blazing/chat/live`, …)
- Tag hosts and filter by name, hostname, folder, or tag with **live search**
- Import and export the host list as a plain YAML file

### SSH terminal
- Launch a full interactive SSH session (`s`) directly from the host list — the TUI steps aside and your shell takes over
- Returns to the manager when you exit

### Dual-pane SFTP browser
- Local panel (left) and remote panel (right), navigate both with arrow keys and Enter
- **Upload / download files and directories** — recursive, with a live progress modal you can cancel mid-transfer
- **View** files in a built-in scrollable viewer (capped at 512 KB)
- **Edit** any file in `$EDITOR` — remote files are downloaded to a temp file, opened, and re-uploaded only if you actually changed them
- **Create** files and directories on either side
- **Delete** files and directories (recursive, with confirmation)
- Open an SSH terminal without leaving the browser (`t` / `Ctrl+O`)

### Authentication
- **SSH agent** (`$SSH_AUTH_SOCK`) — tried first, automatically
- **Private key files** — explicit path or the default set (`id_ed25519`, `id_rsa`, `id_ecdsa`)
- **Password** + keyboard-interactive (both tried)
- If key auth fails, the app prompts for a password and retries once

### Security
- Passwords stored as `enc:<base64>` — Argon2id (t=3, m=64 MB, p=4) for key derivation, AES-256-GCM for encryption
- Master password cached in memory only, never written to disk
- Known-hosts via `~/.ssh/known_hosts`; unknown hosts show a SHA-256 fingerprint and ask before trusting (TOFU)
- Host key changes produce a hard error — no silent downgrade

---

## Install

**From source (recommended until prebuilt releases are available):**

```bash
go install github.com/igorivitskyy/sshmanager/cmd/sshmanager@latest
```

**Build manually:**

```bash
git clone https://github.com/igorivitskyy/sshmanager.git
cd sshmanager
make build        # → ./sshmanager
make install      # → $GOPATH/bin/sshmanager
```

**Prebuilt binaries:** coming soon (tracking in [#1](../../issues)).

**Runtime requirements:**

| Dependency | Used for |
|---|---|
| `ssh` on `$PATH` | Interactive SSH terminal (`s`, `t`) |
| `$EDITOR` or `$VISUAL` | In-app file editing (`e` / F4) — falls back to `nano`, `vim`, `vi` |
| `SSH_AUTH_SOCK` | SSH agent (optional, auto-detected) |

---

## Quick start

```
$ sshmanager
```

| Step | Key | What happens |
|---|---|---|
| Add your first host | `a` | Opens the Add Host form |
| Fill in name, hostname, user | — | `Port` defaults to 22; `Auth Type` defaults to `key` |
| Save | `Save` button | Host appears in the list |
| Open SFTP browser | `Enter` | Connects and opens the dual-pane browser |
| Launch SSH terminal | `s` | TUI suspends, `ssh` starts |
| Quit | `q` | Exits the app |

---

## Configuration

**Location:** `$XDG_CONFIG_HOME/sshmanager/hosts.yaml`  
Falls back to `~/.config/sshmanager/hosts.yaml` when `XDG_CONFIG_HOME` is not set.

File and parent directory are created automatically on first save. Permissions: file `0600`, directory `0700`.

**Example `hosts.yaml`:**

```yaml
version: "1"
hosts:
  # Key-based auth, nested folder, tags
  - id: aae70b507c237a05
    name: web-prod-1
    hostname: 192.168.1.10
    port: 22
    user: deploy
    auth_type: key
    key_path: ~/.ssh/deploy_rsa
    folder: prod/web
    tags: [prod, nginx]

  # Non-standard port
  - id: b1c2d3e4f5a6b7c8
    name: db-prod
    hostname: 192.168.1.20
    port: 2222
    user: root
    auth_type: key
    folder: prod/database

  # Password auth (stored encrypted when a master password is set)
  - id: c2d3e4f5a6b7c8d9
    name: legacy-box
    hostname: 10.0.0.5
    port: 22
    user: admin
    auth_type: password
    password: enc:base64encodedciphertext==
    folder: legacy
```

**Master password:** when you save a host with a password for the first time, the app prompts for a master password. That master password is used to derive an encryption key (Argon2id) and encrypt the stored password (AES-256-GCM). The master password is never written anywhere — you re-enter it once per session when first connecting to a password-auth host.

> ⚠️ If no master password is set, passwords are stored as plaintext in the YAML file. Set a master password before storing any real credentials.

---

## Security details

| Concern | Behaviour |
|---|---|
| Password at rest | Argon2id (t=3, m=64 MB, p=4, 32-byte key) + AES-256-GCM, per-password random salt and nonce |
| Master password | Held in process memory for the session only; zeroized on exit (Go GC) |
| Host key verification | `~/.ssh/known_hosts` via `golang.org/x/crypto/ssh/knownhosts` |
| Unknown host | SHA-256 fingerprint shown; user must explicitly choose **Trust** |
| Host key change | Hard error with MITM warning; user must remove the old entry manually |
| Config file perms | Written as `0600`; parent dir `0700` |

---

## Keyboard shortcuts

### Host list

| Key | Action |
|---|---|
| `a` | Add new host |
| `e` | Edit selected host |
| `d` | Delete selected host (confirmation required) |
| `Enter` | Open SFTP file browser |
| `s` / `S` | Launch interactive SSH terminal |
| `/` | Open live search bar |
| `Esc` | Close search bar |
| `I` | Import hosts from YAML file |
| `E` | Export hosts to YAML file |
| `q` | Quit |
| `Ctrl+Q` | Quit (alternative) |

### File browser

| Key | Action |
|---|---|
| `Tab` / `Shift+Tab` | Switch active panel (local ↔ remote) |
| `Enter` | Open directory / go to parent (`..`) |
| `u` / `U` / `F5` | Upload selected file or directory |
| `d` / `D` / `F6` | Download selected file or directory |
| `v` / `V` / `F3` | View file (read-only) |
| `e` / `E` / `F4` | Edit file in `$EDITOR` |
| `n` / `N` | Create new empty file |
| `m` / `M` / `F7` | Create new directory |
| `x` / `X` / `F8` / `Delete` | Delete file or directory |
| `t` / `T` / `Ctrl+O` | Open SSH terminal |
| `Esc` | Disconnect and return to host list |

### File viewer

| Key | Action |
|---|---|
| `↑` / `↓`, `PgUp` / `PgDn` | Scroll vertically |
| `←` / `→` | Scroll horizontally |
| `Esc` | Close viewer |

---

## Architecture

```
cmd/sshmanager/     CLI entry point — flags, version, TUI bootstrap
internal/
  model/            Host struct, Store, ID generation (no dependencies)
  config/           YAML load/save (0600 perms), Argon2id+AES-GCM crypto, import/export
  ssh/              SFTP client (upload/download/browse/delete, TOFU known_hosts),
                    SSH terminal launcher (shells out to system ssh)
  ui/               tview TUI: host list tree, add/edit form, dual-pane file browser,
                    modal helpers (progress, confirm, error, input)
```

The packages form a strict dependency chain: `model` ← `config` ← `ssh` ← `ui` ← `cmd`. Nothing in `internal/` imports from `cmd/`.

---

## Roadmap — come build with us

sshmanager is functional and actively used, but there is a lot of room to grow. Here are
the things on the wish list — if any of these excite you, open an issue and let's talk.

| Priority | Idea |
|---|---|
| 🔥 High | Prebuilt release binaries (GoReleaser + GitHub Actions) |
| 🔥 High | GitHub Actions CI (`go test`, `go vet`, `go build` on every push) |
| 🔥 High | Homebrew tap |
| ✨ Medium | Support passphrase-protected private keys |
| ✨ Medium | Configurable color themes |
| ✨ Medium | Mouse support |
| ✨ Medium | Transfer queue — see all in-progress up/downloads in one panel |
| ✨ Medium | Parallel multi-file transfers |
| 🧪 Stretch | Port forwarding / SSH tunnel manager |
| 🧪 Stretch | Bulk "run command on all hosts in a folder or tag" |
| 🧪 Stretch | Resume interrupted large-file transfers |
| 🧪 Stretch | Windows support (terminal handling differences) |
| 🧪 Stretch | i18n / localization |

Good first issues are labeled in the issue tracker. The codebase is ~2 700 lines of
straightforward Go — no framework magic, no generated code. If you can read a Go struct,
you can contribute.

---

## Contributing

Pull requests are welcome! Please read [CONTRIBUTING.md](CONTRIBUTING.md) for the full
guide: dev setup, code style, commit conventions, test instructions, and the list of good
first issues.

**Contributor licensing note:** by submitting a PR you agree that your contribution is
made under PolyForm Noncommercial 1.0.0 and that the project maintainer may also include
it in commercially-licensed distributions. You retain copyright in your own work.

---

## License

sshmanager is dual-licensed:

- **Free for non-commercial use** under [PolyForm Noncommercial 1.0.0](LICENSE).  
  This covers personal use, research, education, open-source projects, and non-profit organizations.

- **Commercial use requires a paid license.** See [COMMERCIAL-LICENSE.md](COMMERCIAL-LICENSE.md) for details and contact info.

---

## Acknowledgements

Built on the shoulders of excellent open-source libraries:

- [rivo/tview](https://github.com/rivo/tview) — TUI widget framework
- [gdamore/tcell](https://github.com/gdamore/tcell) — terminal cell rendering
- [pkg/sftp](https://github.com/pkg/sftp) — SFTP client
- [golang.org/x/crypto](https://pkg.go.dev/golang.org/x/crypto) — SSH client, known_hosts, Argon2id
- [gopkg.in/yaml.v3](https://gopkg.in/yaml.v3) — config serialization
