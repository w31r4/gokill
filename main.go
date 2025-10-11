package main

import (
	"os"
	"strings"

	"gkill/internal/tui"
)

func main() {
	var filter string
	if len(os.Args) > 1 {
		filter = strings.Join(os.Args[1:], " ")
	}
	tui.Start(filter)
}
