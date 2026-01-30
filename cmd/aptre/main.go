package main

import (
	"context"
	"fmt"
	"os"

	"github.com/aperturerobotics/cli"
)

// Version is set at build time.
var Version = "dev"

func main() {
	app := &cli.App{
		Name:    "aptre",
		Usage:   "Build tool for Go projects with protobuf support",
		Version: Version,
		Commands: []*cli.Command{
			generateCmd,
			cleanCmd,
			depsCmd,
			lintCmd,
			fixCmd,
			testCmd,
			formatCmd,
			goimportsCmd,
			outdatedCmd,
			releaseCmd,
		},
	}

	if err := app.RunContext(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
