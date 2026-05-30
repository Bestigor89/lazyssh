# LazySSH

**A keyboard-driven SSH manager and SFTP file browser for the terminal — inspired by lazygit.**

[![Go 1.21+](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go)](https://golang.org)
[![License: PolyForm Noncommercial](https://img.shields.io/badge/License-PolyForm_Noncommercial-blue)](LICENSE)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen)](CONTRIBUTING.md)
[![Built with tview](https://img.shields.io/badge/TUI-tview-orange)](https://github.com/rivo/tview)

Manage dozens of remote servers from a single terminal window. Organize SSH connections
into nested folders, search instantly, jump into an SSH shell or a dual-pane SFTP
file browser with one keystroke — all without touching your mouse.

---

![LazySSH — host list with nested folders](img/home.png)

<p align="center">
  <img src="img/addhost.png" width="45%" alt="Add Host form" />
  &nbsp;&nbsp;
  <img src="img/serverpanel.png" width="50%" alt="Dual-pane SFTP browser" />
</p>

---

## Why LazySSH?

If you manage more than a handful of servers, you have felt the pain: scrolling through
`~/.ssh/config`, maintaining shell aliases, or wrestling with a GUI app that does not
belong in a terminal workflow. LazySSH fixes that.

- **Organized at a glance** — group hosts in nested folders (`prod/web`, `staging/db`, …), add tags, filter with live search.
- **SSH + SFTP in one place** — open a shell or a dual-pane file browser without leaving the app.
- **Sessions that survive disconnects** — built-in session manager keeps your work running when the network drops.
- **Passwords encrypted at rest** — Argon2id key derivation + AES-256-GCM encryption. One master password per session.
- **Zero friction** — a single static binary, one YAML file, no daemon, no cloud sync, no Electron.

---

## Features

### SSH connection manager
- Add, edit, delete hosts via a form-based terminal UI
- Group hosts in **nested folders** (unlimited depth) and **tag** them
- **Live search** by name, hostname, folder, or tag
- Import / export the full host list as a plain YAML file

### Persistent sessions
Press `s` to open the session selector instead of a plain shell. LazySSH automatically
deploys a small session helper (`lss`, ~2.5 MB) to the remote server the first time — no
root or package manager needed.

- **Sessions survive disconnects** — close the laptop, lose Wi-Fi, reboot your local machine. The shell and everything running in it stays alive on the server.
- **Multiple sessions per host** — name them anything (`work`, `build`, `logs`). Each one is independent.
- **Session selector** — every time you press `s` a list appears: pick an existing session to resume or create a new one.
- **Detach without closing** — press `Ctrl-\` to leave a session running and return to LazySSH.
- **Zero config on the server** — no tmux, no screen, no pre-installed software required.

```
┌─ Sessions — prod-web ─────────────────┐
│  [+] New session                      │
│      work                             │
│      build                            │
└───────────────────────────────────────┘
```

How it works: `lss` is a purpose-built session daemon written in Go. It allocates a PTY,
starts your shell, and listens on a Unix socket. When you detach or lose connectivity the
shell keeps running. The next `ssh` connection picks up right where you left off.
LazySSH deploys `lss` once via SFTP and reuses it on every subsequent connection.

### One-key SSH terminal
- Press `s` on any host to open the session selector
- The TUI suspends cleanly and hands over the terminal; returns when you exit or detach

### Dual-pane SFTP file browser
- Local panel (left) ↔ remote panel (right), keyboard-navigated
- **Upload / download** files and directories — recursive, with a live progress counter and mid-transfer cancel
- **View** files in a built-in scrollable viewer (capped at 512 KB)
- **Edit** any file in `$EDITOR` — for remote files: download → edit → auto re-upload on save
- **Create** files and directories on either panel
- **Delete** with recursive directory removal and confirmation dialogs
- **Open an SSH session** without leaving the file browser (`t` / `Ctrl+O`)

### Authentication
- **SSH agent** (`$SSH_AUTH_SOCK`) — tried first, automatically
- **Private key files** — explicit path or standard defaults (`id_ed25519`, `id_rsa`, `id_ecdsa`)
- **Password + keyboard-interactive** — prompted only when needed; if key auth fails LazySSH asks for a password and retries once

### Security
- Passwords stored as `enc:<base64>` — [Argon2id](https://en.wikipedia.org/wiki/Argon2) (t=3, m=64 MB, p=4) for key derivation, AES-256-GCM for encryption
- Master password cached in memory only — never written to disk
- Host keys verified via `~/.ssh/known_hosts`; unknown hosts show a **SHA-256 fingerprint** and require explicit trust (TOFU)
- Host key changes produce a hard error with an MITM warning — no silent downgrade

---

## Install

### macOS — Homebrew

```bash
brew tap Bestigor89/tap
brew install lazyssh
```

### Linux — prebuilt packages

Download `.deb` (Ubuntu/Debian) or `.rpm` (RHEL/Fedora) from the
[releases page](https://github.com/Bestigor89/lazyssh/releases/latest):

```bash
# Ubuntu / Debian
sudo dpkg -i lazyssh_*_amd64.deb

# RHEL / Fedora / CentOS
sudo rpm -i lazyssh_*_x86_64.rpm
```

### Any OS — Go install

```bash
go install github.com/Bestigor89/lazyssh/cmd/lazyssh@latest
```

### Build from source

```bash
git clone https://github.com/Bestigor89/lazyssh.git
cd lazyssh
make build-full  # builds lss helpers then lazyssh with sessions embedded
make install     # → $GOPATH/bin/lazyssh
```

### Prebuilt binaries

Download the archive for your platform from the
[releases page](https://github.com/Bestigor89/lazyssh/releases/latest),
extract and put `lazyssh` anywhere on your `$PATH`.

| Platform | File |
|---|---|
| macOS Apple Silicon | `lazyssh_*_darwin_arm64.tar.gz` |
| macOS Intel | `lazyssh_*_darwin_amd64.tar.gz` |
| Linux x86-64 | `lazyssh_*_linux_amd64.tar.gz` |
| Linux ARM64 | `lazyssh_*_linux_arm64.tar.gz` |
| Windows | `lazyssh_*_windows_amd64.zip` |

**Requirements at runtime:**

| Dependency | Used for |
|---|---|
| `ssh` on `$PATH` | Interactive SSH terminal (`s`, `t`) |
| `$EDITOR` / `$VISUAL` | In-app file editing — falls back to `nano`, `vim`, `vi` |
| `SSH_AUTH_SOCK` | SSH agent (optional, auto-detected) |

The `lss` session helper has **no runtime dependencies** — it is deployed automatically
to Linux servers (amd64 / arm64) the first time you open a session.

---

## Quick start

```
$ lazyssh
```

| Step | Key | What happens |
|---|---|---|
| Add your first host | `a` | Opens the Add Host form |
| Fill in name, hostname, user | — | Port defaults to 22; Auth Type defaults to `key` |
| Save | `Save` button | Host appears in the list |
| Open session selector | `s` | Lists existing sessions or lets you create a new one |
| Open SFTP browser | `Enter` | Connects and opens the dual-pane browser |
| Quit | `q` | Exits the app |

---

## Configuration

**Config file:** `$XDG_CONFIG_HOME/lazyssh/hosts.yaml`  
Falls back to `~/.config/lazyssh/hosts.yaml`.  
Created automatically on first save. Permissions: `0600` (file), `0700` (directory).

**Example `hosts.yaml`:**

```yaml
version: "1"
hosts:
  # Key-based auth with nested folder and tags
  - id: aae70b507c237a05
    name: web-prod-1
    hostname: 192.168.1.10
    port: 22
    user: deploy
    auth_type: key
    key_path: ~/.ssh/deploy_rsa
    folder: prod/web
    tags: [prod, nginx]

  # Non-standard SSH port
  - id: b1c2d3e4f5a6b7c8
    name: db-prod
    hostname: 192.168.1.20
    port: 2222
    user: root
    auth_type: key
    folder: prod/database

  # Password auth (encrypted when a master password is set)
  - id: c2d3e4f5a6b7c8d9
    name: legacy-box
    hostname: 10.0.0.5
    port: 22
    user: admin
    auth_type: password
    password: enc:base64encodedciphertext==
    folder: legacy
```

**Master password:** the first time you save a host with a password, LazySSH prompts for a master password. That password derives an encryption key via Argon2id and protects the stored credential with AES-256-GCM. It is never persisted — you enter it once per session.

> ⚠️ Without a master password, passwords are stored in plaintext. Set a master password before adding real credentials.

---

## Persistent sessions in depth

When you press `s`, LazySSH:

1. Opens an SFTP connection to the host.
2. Checks whether `~/.lazyssh/bin/lss` exists on the server.
3. If not — asks permission and uploads the embedded `lss` binary (~2.5 MB, no root needed).
4. Runs `lss list` to fetch active sessions.
5. Shows the session selector. Pick one or create a new one.
6. Runs `ssh -t host lss new <name>` or `lss attach <name>` — you land directly in your shell.

**Session files live in `~/.lazyssh/sessions/` on the remote server.** Each session is a
Unix socket + a PID file. Stale entries (process died) are cleaned up automatically on
the next `lss list`.

**Supported remote platforms:** Linux x86-64 and ARM64. On macOS servers or other
architectures LazySSH falls back to a plain SSH shell automatically.

| Scenario | Result |
|---|---|
| Network drops mid-session | Shell keeps running; reconnect and reattach |
| Press `Ctrl-\` | Detach — session stays alive, TUI resumes |
| Type `exit` in shell | Session closes normally |
| Run `lss kill <name>` on server | Session terminated |

---

## Security details

| Concern | Behaviour |
|---|---|
| Password at rest | Argon2id (t=3, m=64 MB, p=4, 32-byte key) + AES-256-GCM; random salt and nonce per password |
| Master password | Memory-only for the session; never written to disk |
| Host key verification | `~/.ssh/known_hosts` via `golang.org/x/crypto/ssh/knownhosts` |
| Unknown host | SHA-256 fingerprint shown; requires explicit **Trust** |
| Host key change | Hard error + MITM warning; old entry must be removed manually |
| Config permissions | File `0600`, directory `0700` |
| lss binary | Deployed to user home only (`~/.lazyssh/`); no elevated privileges |

---

## Keyboard shortcuts

### Host list

| Key | Action |
|---|---|
| `a` | Add new host |
| `e` | Edit selected host |
| `d` | Delete selected host (confirmation required) |
| `Enter` | Open SFTP file browser |
| `s` / `S` | Open session selector (persistent sessions) |
| `/` | Open live search / filter bar |
| `Esc` | Close search bar |
| `I` | Import hosts from a YAML file |
| `E` | Export hosts to a YAML file |
| `q` | Quit |
| `Ctrl+Q` | Quit (alternative) |

### Session selector

| Key | Action |
|---|---|
| `↑` / `↓` | Navigate sessions |
| `Enter` | Attach to selected session / create new |
| `Esc` | Cancel and return to host list |

### Inside a session

| Key | Action |
|---|---|
| `Ctrl-\` | Detach — session stays running, return to LazySSH |
| `exit` | Close session normally |

### File browser

| Key | Action |
|---|---|
| `Tab` / `Shift+Tab` | Switch active panel (local ↔ remote) |
| `Enter` | Open directory / `..` goes to parent |
| `u` / `U` / `F5` | Upload selected file or directory |
| `d` / `D` / `F6` | Download selected file or directory |
| `v` / `V` / `F3` | View file in built-in viewer |
| `e` / `E` / `F4` | Edit file in `$EDITOR` |
| `n` / `N` | Create new empty file |
| `m` / `M` / `F7` | Create new directory |
| `x` / `X` / `F8` / `Delete` | Delete file or directory |
| `t` / `T` / `Ctrl+O` | Open session selector |
| `Esc` | Disconnect and return to host list |

### File viewer

| Key | Action |
|---|---|
| `↑` `↓` `PgUp` `PgDn` | Scroll vertically |
| `←` `→` | Scroll horizontally |
| `Esc` | Close viewer |

---

## Architecture

```
cmd/
  lazyssh/      CLI entry point: flags, version, TUI bootstrap
  lss/          Session helper daemon (runs on remote Linux servers)
                  daemon.go  — PTY allocation, Unix socket server
                  client.go  — raw terminal I/O, resize forwarding, detach
                  proto.go   — simple length-prefixed frame protocol
                  main.go    — new / attach / list / kill subcommands
internal/
  model/        Host struct, Store, ID generation
  config/       YAML load/save (0600), Argon2id + AES-GCM crypto, import/export
  ssh/          SFTP client (upload/download/browse/delete, known_hosts TOFU)
                SSH terminal launcher (shells out to system ssh)
                Session manager (deploy lss, list sessions, build remote commands)
                Embedded lss binaries for linux/amd64 and linux/arm64
  ui/           tview TUI: host list, host form, dual-pane file browser,
                session selector, modals
```

Strict dependency order: `model` ← `config` ← `ssh` ← `ui` ← `cmd`. Nothing in `internal/` imports from `cmd/`.

---

## Roadmap — come build this with us

**Medium scope:**
- [ ] Support passphrase-protected private keys (prompt at connect time)
- [ ] Configurable color themes
- [ ] Transfer queue — a panel showing all in-progress uploads and downloads
- [ ] Parallel multi-file transfers

**Larger features:**
- [ ] Port forwarding / SSH tunnel manager
- [ ] Bulk command — run a shell command across all hosts in a folder or tag group
- [ ] Resume interrupted large-file transfers
- [ ] Windows support (mostly terminal-handling differences)
- [ ] i18n / localization

The codebase is ~3 500 lines of idiomatic Go with no framework magic. If you have read a Go struct, you can contribute. **Good first issues are labeled in the issue tracker.**

---

## Similar tools

LazySSH fills a niche between lightweight `~/.ssh/config` managers and full GUI clients.
You might also find these useful:

| Tool | What it is |
|---|---|
| [lazygit](https://github.com/jesseduffield/lazygit) | The original lazy* TUI — for Git |
| [sshs](https://github.com/quantumsheep/sshs) | Terminal UI for `~/.ssh/config` |
| [sshx](https://sshx.io) | Web-based collaborative terminal sharing |
| [Termius](https://termius.com) | GUI SSH client with SFTP (commercial) |
| [FileZilla](https://filezilla-project.org) | GUI SFTP/FTP client |

LazySSH's focus: **keyboard-first, terminal-native, encrypted config, persistent sessions, SFTP built in, one binary.**

---

## Contributing

Pull requests are welcome. Read [CONTRIBUTING.md](CONTRIBUTING.md) for the full guide:
dev setup, code style, commit conventions, test instructions, and the list of good first issues.

> **Contributor licensing note:** by submitting a PR you agree your contribution is licensed under PolyForm Noncommercial 1.0.0 and that the project maintainer may include it in commercially-licensed distributions. You retain copyright in your own work.

---

## License

LazySSH is dual-licensed:

- **Free for non-commercial use** under [PolyForm Noncommercial 1.0.0](LICENSE) — personal use, research, education, open-source projects, non-profit organizations.
- **Commercial use requires a paid license.** See [COMMERCIAL-LICENSE.md](COMMERCIAL-LICENSE.md) for details and contact information.

---

## Acknowledgements

Built with excellent open-source libraries:

- [rivo/tview](https://github.com/rivo/tview) — TUI widget framework
- [gdamore/tcell](https://github.com/gdamore/tcell) — terminal cell rendering
- [pkg/sftp](https://github.com/pkg/sftp) — SFTP client
- [golang.org/x/crypto](https://pkg.go.dev/golang.org/x/crypto) — SSH client, known_hosts, Argon2id
- [gopkg.in/yaml.v3](https://gopkg.in/yaml.v3) — config serialization
- [creack/pty](https://github.com/creack/pty) — PTY allocation for the session helper

---

<!-- Search keywords (for indexers):
ssh manager tui, sftp client terminal, keyboard driven ssh, go tui ssh manager,
terminal sftp file browser, lazygit for ssh, ssh connection organizer, remote server manager,
cli ssh manager, ncurses ssh, ssh bookmark manager, sftp tui go, terminal ssh client,
dual pane file manager ssh, scp client terminal, winscp alternative terminal,
ssh connection manager linux mac, ssh manager encrypted, sftp go cli,
persistent ssh sessions, ssh session manager, tmux alternative, ssh reconnect
-->
