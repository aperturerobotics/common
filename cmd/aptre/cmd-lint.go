package main

import (
	"os"
	"os/exec"

	"github.com/aperturerobotics/cli"
)

var lintCmd = &cli.Command{
	Name:  "lint",
	Usage: "Run golangci-lint",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "tools-dir",
			Usage: "Tools directory, relative to the project or absolute to share it between checkouts",
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

// runLint runs the selected linter with the caller's arguments.
func runLint(c *cli.Context) error {
	// Read the command's project and tool-directory options.
	projectDir := c.String("project-dir")
	toolsDir := c.String("tools-dir")
	verbose := c.Bool("verbose")

	// Default the project to the invocation directory.
	if projectDir == "" {
		var err error
		projectDir, err = os.Getwd()
		if err != nil {
			return err
		}
	}

	// Reuse or build the tool selected by this project.
	lintPath, err := EnsureToolBuilt(projectDir, toolsDir, "golangci-lint", verbose)
	if err != nil {
		return err
	}

	// Preserve the caller's lint arguments after the command's fixed mode.
	args := []string{"run"}
	args = append(args, c.Args().Slice()...)

	// Forward the tool's output and exit status without translation.
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
			Usage: "Tools directory, relative to the project or absolute to share it between checkouts",
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

// runFix runs the selected linter with fixes enabled.
func runFix(c *cli.Context) error {
	// Read the command's project and tool-directory options.
	projectDir := c.String("project-dir")
	toolsDir := c.String("tools-dir")
	verbose := c.Bool("verbose")

	// Default the project to the invocation directory.
	if projectDir == "" {
		var err error
		projectDir, err = os.Getwd()
		if err != nil {
			return err
		}
	}

	// Reuse or build the tool selected by this project.
	lintPath, err := EnsureToolBuilt(projectDir, toolsDir, "golangci-lint", verbose)
	if err != nil {
		return err
	}

	// Preserve the caller's lint arguments after the command's fixed mode.
	args := []string{"run", "--fix"}
	args = append(args, c.Args().Slice()...)

	// Forward the tool's output and exit status without translation.
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
			Usage: "Tools directory, relative to the project or absolute to share it between checkouts",
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

// runFormat formats the selected project path with its configured formatter.
func runFormat(c *cli.Context) error {
	// Read the command's project and tool-directory options.
	projectDir := c.String("project-dir")
	toolsDir := c.String("tools-dir")
	verbose := c.Bool("verbose")

	// Default the project to the invocation directory.
	if projectDir == "" {
		var err error
		projectDir, err = os.Getwd()
		if err != nil {
			return err
		}
	}

	// Reuse or build the tool selected by this project.
	gofumptPath, err := EnsureToolBuilt(projectDir, toolsDir, "gofumpt", verbose)
	if err != nil {
		return err
	}

	// An explicit target narrows the command from the whole project.
	target := "./"
	if c.NArg() > 0 {
		target = c.Args().First()
	}

	// Forward the tool's output and exit status without translation.
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
			Usage: "Tools directory, relative to the project or absolute to share it between checkouts",
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

// runGoimports rewrites imports for the selected project path.
func runGoimports(c *cli.Context) error {
	// Read the command's project and tool-directory options.
	projectDir := c.String("project-dir")
	toolsDir := c.String("tools-dir")
	verbose := c.Bool("verbose")

	// Default the project to the invocation directory.
	if projectDir == "" {
		var err error
		projectDir, err = os.Getwd()
		if err != nil {
			return err
		}
	}

	// Reuse or build the tool selected by this project.
	goimportsPath, err := EnsureToolBuilt(projectDir, toolsDir, "goimports", verbose)
	if err != nil {
		return err
	}

	// An explicit target narrows the command from the whole project.
	target := "./"
	if c.NArg() > 0 {
		target = c.Args().First()
	}

	// Forward the tool's output and exit status without translation.
	cmd := exec.Command(goimportsPath, "-w", target)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
