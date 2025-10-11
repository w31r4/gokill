package main

import (
	"os"
	"strings"

	"github.com/w31r4/gokill/internal/tui"
)

func main() {
	var filter string
	if len(os.Args) > 1 {
		filter = strings.Join(os.Args[1:], " ")
	}
	tui.Start(filter)
}
