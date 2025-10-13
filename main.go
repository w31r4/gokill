package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/w31r4/gokill/internal/process"
	"github.com/w31r4/gokill/internal/tui"
)

func main() {
	var filter string
	if len(os.Args) > 1 {
		filter = strings.Join(os.Args[1:], " ")
	}

	// Pre-load processes synchronously
	procs, err := process.GetProcesses()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting processes: %v\n", err)
		os.Exit(1)
	}

	tui.Start(filter, procs)
}
