package main

import (
	"os"

	"github.com/CarlosHPlata/shrine/cmd"
)

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func main() {
	cmd.Version = Version
	cmd.Commit = Commit
	cmd.Date = Date

	err := cmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
