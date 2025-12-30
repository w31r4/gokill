# gokill

**A modern, interactive process killer for macOS & Linux with "Why Is This Running" analysis.**

[![Release](https://img.shields.io/github/v/release/w31r4/gokill?style=flat-square)](https://github.com/w31r4/gokill/releases)
[![npm](https://img.shields.io/npm/v/@zenfun510/gokill?style=flat-square)](https://www.npmjs.com/package/@zenfun510/gokill)
[![Go Report Card](https://goreportcard.com/badge/github.com/w31r4/gokill?style=flat-square)](https://goreportcard.com/report/github.com/w31r4/gokill)

This project is a complete rewrite and modernization of the original [gkill](https://github.com/heppu/gkill), rebuilt from the ground up with modern Go, [Bubble Tea](https://github.com/charmbracelet/bubbletea), and fuzzy search capabilities.

For a Chinese version of this document, see: `README_zh.md`.

## Why Is This Running? (Powered by witr)

gokill integrates **process ancestry analysis** inspired by [witr (why-is-this-running)](https://github.com/pranshuparmar/witr), helping you understand not just *what* is running, but *why* it exists.

### Key Capabilities

| Feature | Description |
|---------|-------------|
| **Process Ancestry Chain** | Traces the full parent chain from init/systemd to target process |
| **Source Detection** | Identifies the supervisor/launcher (systemd, launchd, Docker, PM2, supervisor, cron, shell) |
| **Container Awareness** | Detects if a process runs inside Docker, containerd, Kubernetes, or LXC |
| **Git Context** | Shows the Git repository and branch when a process runs from a Git directory |
| **Health Warnings** | Alerts for zombie processes, root execution, high memory usage, long-running processes |

### Example: Process Details View

When you press `i` on a selected process, gokill shows:

```
PID       : 14233
User      : pm2
Command   : node index.js
Started   : 2 days ago

Why It Exists:
  systemd (pid 1) → pm2 (pid 5034) → node (pid 14233)

Source    : pm2
Git Repo  : expense-manager (main)
Warnings  : Process is running as root
```

This is especially powerful during incident response when you need to quickly understand the chain of responsibility for a running process.

## Installation

### Homebrew (macOS/Linux)

```sh
brew install w31r4/tap/gokill
```

### npm

```sh
npm install -g @zenfun510/gokill
```

### Go Install

Ensure you have a working Go environment. You can install `gokill` with `go install`:

```sh
go install github.com/w31r4/gokill@latest
```

### Build from Source

Alternatively, you can clone the repository and build it from source:

```sh
git clone https://github.com/w31r4/gokill.git
cd gokill
go build
```

## Usage

Run `gokill` in your terminal to start the interactive interface. You can immediately start typing to fuzzy search for processes by name, PID, username, or ports.

### Keybindings

| Key | Action |
| --- | --- |
| `up`/`k` | Move cursor up |
| `down`/`j` | Move cursor down |
| `/` | Enter search/filter mode |
| `enter` | Kill selected process (in navigation mode) / Exit search mode |
| `esc` | Exit search mode / Close overlays (details, error, ports-only, dependency tree, help) |
| `p` | Pause selected process (SIGSTOP) |
| `r` | Resume selected process (SIGCONT) |
| `i` | Show process details |
| `P` | Toggle ports-only view |
| `T` | Open dependency tree (T-mode) for the selected process |
| `?` | Open contextual help overlay for the current mode |
| `ctrl+r` | Refresh process list |
| `q`/`ctrl+c` | Quit |

### Details Mode

Press `i` on a selected process to open a details view showing PID, user, CPU/MEM, start time, and command. Press `esc` to return to the list once you are done.

### Ports-only View

Press `P` (uppercase) to show only processes that are listening on ports. In this view, the list is sorted by the smallest port number in ascending order.

### Port Scanning (optional)

By default, gokill scans listening ports for processes displayed in the list and in the details view. This can be slow or require elevated privileges on some systems. To disable port scanning, set:

```sh
export GOKILL_SCAN_PORTS=0
```

When disabled, the list won’t highlight listeners and the details view won’t include the Ports line.

You can tune the port scan timeout (per process) via `GOKILL_PORT_TIMEOUT_MS` (default 300):

```sh
export GOKILL_PORT_TIMEOUT_MS=200
```

### Dependency Tree Mode (T-mode)

Press `T` on a selected process to enter a full-screen dependency tree view rooted at that process. In T-mode:

- Use `up`/`down` (`j`/`k`) to move the cursor.
- Use `left`/`right` (`h`/`l`) or `space` to fold/unfold branches.  
  When the cursor is on a `… (deeper)` line, pressing `right`/`l` or `space` drills into a deeper level of the subtree;  
  when it is on a `… (N more)` line, pressing `right`/`l` or `space` pages through additional siblings at the same level.
- Press `enter`/`o` to make the selected node the new root; `u` moves the root up to its parent.
- Press `/` to filter the tree by text or PID; `S` toggles “alive-only” and `L` toggles “listening-only”.
- Press `i` to open details for the selected node, or `x`/`p`/`r` to kill, pause, or resume that node (with a confirmation prompt).
- Press `esc` to leave T-mode and return to the main list.

### Help Overlay

Press `?` at any time to open a help overlay summarizing the available keybindings for the current view (main list or T-mode). Press `?` or `esc` again to close it.

## Common Errors & Remedies

| Error message | When it appears | Suggested fix |
| --- | --- | --- |
| `operation not permitted` | You tried to send a signal to a root/protected process without enough privileges. | Run `gokill` with `sudo` or target processes owned by your user. |
| `process with pid XXX not found` | The process exited (or the PID was reassigned) before the signal landed. | Press `ctrl+r` to refresh the list and pick another process. |
| `failed to get user/create time/...` (shown in warnings) | `gopsutil` could not read that attribute. | Usually safe to ignore; running with higher privileges can reduce these warnings. |
| `connection scan timeout` (with `GOKILL_SCAN_PORTS` enabled) | The port scan took too long or was blocked by a firewall. | Increase `GOKILL_PORT_TIMEOUT_MS` or disable port scanning. |

The error screen appears as a red pane with the prompt `esc: dismiss • q: quit`, so you can return to the main view without quitting the program.

## Related

- [gkill](https://github.com/heppu/gkill) - The original project that this is a modernization of.
- [fkill-cli](https://github.com/sindresorhus/fkill-cli) - A great Node.js alternative that inspired the project.
- [witr](https://github.com/pranshuparmar/witr) - "Why Is This Running" - the project that inspired our process ancestry analysis.

## Acknowledgments

Special thanks to [**witr (why-is-this-running)**](https://github.com/pranshuparmar/witr) by [@pranshuparmar](https://github.com/pranshuparmar). The `internal/why` package in gokill is heavily inspired by witr's core concept of building causal chains to explain why a process exists. witr's philosophy of making process causality explicit—answering not just *what* is running but *why*—directly shaped our process details and ancestry analysis features.

> *"When something is running on a system, there is always a cause. witr makes that causality explicit."*

## License

MIT
