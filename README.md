# gkill

An interactive process killer for Linux and macOS, refactored with modern Go and Bubble Tea.

## Installation

To install `gkill`, make sure you have Go installed on your system. Then, you can use `go install`:

```sh
go install github.com/heppu/gkill@latest
```

Alternatively, you can clone the repository and build it from source:

```sh
git clone https://github.com/heppu/gkill.git
cd gkill
go build
```

## Usage

Run `gkill` in your terminal.

### Keybindings

| Key | Action |
| --- | --- |
| `up`/`k` | Move cursor up |
| `down`/`j` | Move cursor down |
| `/` | Enter filter mode |
| `enter` | Kill selected process (in navigation mode) / Exit filter mode |
| `esc` | Exit filter mode |
| `p` | Pause selected process (SIGSTOP) |
| `r` | Resume selected process (SIGCONT) |
| `ctrl+r` | Refresh process list |
| `q`/`ctrl+c` | Quit |

## Related

- [fkill-cli](https://github.com/sindresorhus/fkill-cli) - A great Node.js alternative that inspired this project.

## License

MIT
