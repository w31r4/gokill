# gokill

**A modern, interactive process killer for macOS & Linux.**

This project is a complete rewrite and modernization of the original [gkill](https://github.com/heppu/gkill), rebuilt from the ground up with modern Go, [Bubble Tea](https://github.com/charmbracelet/bubbletea), and fuzzy search capabilities.

## Installation

Ensure you have a working Go environment. You can install `gokill` with `go install`:

```sh
go install github.com/w31r4/gokill@latest
```

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
| `esc` | Exit search mode / Close overlays (details, error, ports-only) |
| `p` | Pause selected process (SIGSTOP) |
| `r` | Resume selected process (SIGCONT) |
| `i` | Show process details |
| `P` | Toggle ports-only view |
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

## License

MIT
