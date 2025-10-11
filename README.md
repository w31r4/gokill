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

Run `gkill` in your terminal. Use the arrow keys (`up`/`down` or `j`/`k`) to navigate the process list. Type to filter the processes in real-time. Press `enter` to kill the selected process. Press `q` or `ctrl+c` to quit.

## Related

- [fkill-cli](https://github.com/sindresorhus/fkill-cli) - A great Node.js alternative that inspired this project.

## License

MIT
