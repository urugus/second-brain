package main

import (
	"os"

	"github.com/urugus/second-brain/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
