package main

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/aperturerobotics/cli"
)

var lintCmd = &cli.Command{
	Name:  "lint",
	Usage: "Run golangci-lint",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "tools-dir",
			Usage: "Tools directory path",
			Value: ".tools",
		},
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
	},
	Action: runLint,
}

func runLint(c *cli.Context) error {
	projectDir := c.String("project-dir")
	toolsDir := c.String("tools-dir")
	verbose := c.Bool("verbose")

	if projectDir == "" {
		var err error
		projectDir, err = os.Getwd()
		if err != nil {
			return err
		}
	}

	lintPath, err := EnsureToolBuilt(projectDir, toolsDir, "golangci-lint", verbose)
	if err != nil {
		return err
	}

	args := []string{"run"}
	args = append(args, c.Args().Slice()...)

	cmd := exec.Command(lintPath, args...)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

var fixCmd = &cli.Command{
	Name:  "fix",
	Usage: "Run golangci-lint with --fix",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "tools-dir",
			Usage: "Tools directory path",
			Value: ".tools",
		},
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
	},
	Action: runFix,
}

func runFix(c *cli.Context) error {
	projectDir := c.String("project-dir")
	toolsDir := c.String("tools-dir")
	verbose := c.Bool("verbose")

	if projectDir == "" {
		var err error
		projectDir, err = os.Getwd()
		if err != nil {
			return err
		}
	}

	lintPath, err := EnsureToolBuilt(projectDir, toolsDir, "golangci-lint", verbose)
	if err != nil {
		return err
	}

	args := []string{"run", "--fix"}
	args = append(args, c.Args().Slice()...)

	cmd := exec.Command(lintPath, args...)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

var formatCmd = &cli.Command{
	Name:    "format",
	Aliases: []string{"fmt", "gofumpt"},
	Usage:   "Format code with gofumpt",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "tools-dir",
			Usage: "Tools directory path",
			Value: ".tools",
		},
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
	},
	Action: runFormat,
}

func runFormat(c *cli.Context) error {
	projectDir := c.String("project-dir")
	toolsDir := c.String("tools-dir")
	verbose := c.Bool("verbose")

	if projectDir == "" {
		var err error
		projectDir, err = os.Getwd()
		if err != nil {
			return err
		}
	}

	gofumptPath, err := EnsureToolBuilt(projectDir, toolsDir, "gofumpt", verbose)
	if err != nil {
		return err
	}

	target := "./"
	if c.NArg() > 0 {
		target = c.Args().First()
	}

	cmd := exec.Command(gofumptPath, "-w", target)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

var goimportsCmd = &cli.Command{
	Name:  "goimports",
	Usage: "Run goimports",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "tools-dir",
			Usage: "Tools directory path",
			Value: ".tools",
		},
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
	},
	Action: runGoimports,
}

func runGoimports(c *cli.Context) error {
	projectDir := c.String("project-dir")
	toolsDir := c.String("tools-dir")
	verbose := c.Bool("verbose")

	if projectDir == "" {
		var err error
		projectDir, err = os.Getwd()
		if err != nil {
			return err
		}
	}

	goimportsPath := filepath.Join(projectDir, toolsDir, "bin", "goimports")
	if err := ensureTool(filepath.Join(projectDir, toolsDir), "goimports", false, verbose); err != nil {
		return err
	}

	target := "./"
	if c.NArg() > 0 {
		target = c.Args().First()
	}

	cmd := exec.Command(goimportsPath, "-w", target)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
