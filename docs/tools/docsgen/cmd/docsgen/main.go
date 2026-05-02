package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/CarlosHPlata/shrine/cmd"
	"github.com/CarlosHPlata/shrine/docs/tools/docsgen"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("docsgen", flag.ContinueOnError)
	out := fs.String("out", "", "Output directory for generated Markdown files (required)")
	clean := fs.Bool("clean", false, "Remove existing *.md files (except _index.md) before generating")
	includeHidden := fs.Bool("include-hidden", false, "Include hidden Cobra commands in the output")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: docsgen -out <dir> [-clean] [-include-hidden]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		fs.Usage()
		return fmt.Errorf("-out is required")
	}
	return docsgen.Generate(cmd.RootCmd(), *out, docsgen.Options{
		Clean:         *clean,
		IncludeHidden: *includeHidden,
	})
}
