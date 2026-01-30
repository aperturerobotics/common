package main

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/aperturerobotics/cli"
)

var testCmd = &cli.Command{
	Name:  "test",
	Usage: "Run go test",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "project-dir",
			Aliases: []string{"C"},
			Usage:   "Project directory",
		},
		&cli.BoolFlag{
			Name:    "verbose",
			Aliases: []string{"v"},
			Usage:   "Enable verbose output",
		},
		&cli.BoolFlag{
			Name:  "browser",
			Usage: "Run browser/WASM tests",
		},
		&cli.StringFlag{
			Name:  "tools-dir",
			Usage: "Tools directory path",
			Value: ".tools",
		},
	},
	Action: runTest,
}

func runTest(c *cli.Context) error {
	projectDir := c.String("project-dir")
	verbose := c.Bool("verbose")
	browser := c.Bool("browser")
	toolsDir := c.String("tools-dir")

	if projectDir == "" {
		var err error
		projectDir, err = os.Getwd()
		if err != nil {
			return err
		}
	}

	if browser {
		return runBrowserTest(projectDir, toolsDir, verbose, c.Args().Slice())
	}

	args := []string{"test"}
	if verbose {
		args = append(args, "-v")
	}

	// Add extra args or default to ./...
	if c.NArg() > 0 {
		args = append(args, c.Args().Slice()...)
	} else {
		args = append(args, "./...")
	}

	cmd := exec.Command("go", args...)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	return cmd.Run()
}

func runBrowserTest(projectDir, toolsDir string, verbose bool, extraArgs []string) error {
	// Ensure wasmbrowsertest is built
	wasmTestPath, err := EnsureToolBuilt(projectDir, toolsDir, "wasmbrowsertest", verbose)
	if err != nil {
		return err
	}

	// Get absolute path for exec
	absWasmTestPath, err := filepath.Abs(wasmTestPath)
	if err != nil {
		return err
	}

	args := []string{"test", "-exec", absWasmTestPath, "-tags", "webtests"}
	if verbose {
		args = append(args, "-v")
	}

	// Add extra args or default to ./...
	if len(extraArgs) > 0 {
		args = append(args, extraArgs...)
	} else {
		args = append(args, "./...")
	}

	cmd := exec.Command("go", args...)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"GOOS=js",
		"GOARCH=wasm",
		"GOTOOLCHAIN=local",
		"GO111MODULE=on",
	)
	return cmd.Run()
}
