//go:build !linux

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("Geyser sidecar runner is designed to run inside Linux containers.")
	os.Exit(0)
}
