package main

import (
	"os"
	"os/exec"

	"github.com/aperturerobotics/cli"
)

var outdatedCmd = &cli.Command{
	Name:  "outdated",
	Usage: "Show outdated dependencies",
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
		&cli.BoolFlag{
			Name:  "direct",
			Usage: "Show only direct dependencies",
			Value: true,
		},
		&cli.BoolFlag{
			Name:  "update",
			Usage: "Show only dependencies with updates available",
			Value: true,
		},
	},
	Action: runOutdated,
}

func runOutdated(c *cli.Context) error {
	// Read the project and tool selection.
	projectDir := c.String("project-dir")
	toolsDir := c.String("tools-dir")
	verbose := c.Bool("verbose")
	direct := c.Bool("direct")
	update := c.Bool("update")

	// Default the project to the current directory.
	if projectDir == "" {
		var err error
		projectDir, err = os.Getwd()
		if err != nil {
			return err
		}
	}

	// Build the dependency report tool before connecting the pipeline.
	outdatedPath, err := EnsureToolBuilt(projectDir, toolsDir, "go-mod-outdated", verbose)
	if err != nil {
		return err
	}

	// Run: go list -mod=mod -u -m -json all | go-mod-outdated [flags]
	listCmd := exec.Command("go", "list", "-mod=mod", "-u", "-m", "-json", "all")
	listCmd.Dir = projectDir
	listCmd.Env = append(os.Environ(), "GO111MODULE=on")

	// Select which dependencies the report includes.
	outdatedArgs := []string{}
	if update {
		outdatedArgs = append(outdatedArgs, "-update")
	}
	if direct {
		outdatedArgs = append(outdatedArgs, "-direct")
	}

	// Send the filtered report to the terminal.
	outdatedExec := exec.Command(outdatedPath, outdatedArgs...)
	outdatedExec.Dir = projectDir
	outdatedExec.Stdout = os.Stdout
	outdatedExec.Stderr = os.Stderr

	// Pipe go list output to go-mod-outdated
	pipe, err := listCmd.StdoutPipe()
	if err != nil {
		return err
	}
	outdatedExec.Stdin = pipe
	listCmd.Stderr = os.Stderr

	// Start the producer before its report consumer.
	if err := listCmd.Start(); err != nil {
		return err
	}
	if err := outdatedExec.Start(); err != nil {
		return err
	}

	// Reap both processes after the report consumes the module list.
	listCmd.Wait()
	return outdatedExec.Wait()
}
