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

Run `gokill` in your terminal to start the interactive interface. You can immediately start typing to fuzzy search for processes by name or PID.

### Keybindings

| Key | Action |
| --- | --- |
| `up`/`k` | Move cursor up |
| `down`/`j` | Move cursor down |
| `/` | Enter search/filter mode |
| `enter` | Kill selected process (in navigation mode) / Exit search mode |
| `esc` | Exit search mode |
| `p` | Pause selected process (SIGSTOP) |
| `r` | Resume selected process (SIGCONT) |
| `i` | Show process details |
| `ctrl+r` | Refresh process list |
| `q`/`ctrl+c` | Quit |

### Details Mode

Press `i` on a selected process to open a details view showing PID, user, CPU/MEM, start time, and command. Press `q`, `esc`, or `i` again to return.

### Port Scanning (optional)

By default, gokill scans listening ports for processes displayed in the list and in the details view. This can be slow or require elevated privileges on some systems. To disable port scanning, set:

```sh
export GOKILL_SCAN_PORTS=0
```

When disabled, the list won’t highlight listeners and the details view won’t include the Ports line.

## Related

- [gkill](https://github.com/heppu/gkill) - The original project that this is a modernization of.
- [fkill-cli](https://github.com/sindresorhus/fkill-cli) - A great Node.js alternative that inspired the project.

## License

MIT
