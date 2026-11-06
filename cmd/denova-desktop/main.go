package main

import (
	"fmt"
	"os"

	"denova/internal/desktop"
)

func main() {
	if err := desktop.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "Desktop failed:", err)
		os.Exit(1)
	}
}
