package main

import (
	"os"
	"github.com/CarlosHPlata/shrine/cmd"
)

func main() {
	err := cmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}