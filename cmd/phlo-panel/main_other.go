//go:build !windows

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "phlo-panel is the Windows window. On Linux, run ./Phlo.sh")
	os.Exit(1)
}
