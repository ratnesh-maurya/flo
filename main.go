package main

import (
	"os"

	"github.com/ratnesh-maurya/flo/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
