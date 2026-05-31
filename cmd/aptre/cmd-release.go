package main

import (
	"os"
	"os/exec"

	"github.com/aperturerobotics/cli"
)

var releaseCmd = &cli.Command{
	Name:  "release",
	Usage: "Release commands (goreleaser)",
	Subcommands: []*cli.Command{
		releaseRunCmd,
		releaseBundleCmd,
		releaseBuildCmd,
		releaseCheckCmd,
	},
}

var releaseRunCmd = &cli.Command{
	Name:  "run",
	Usage: "Run goreleaser release",
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
	Action: func(c *cli.Context) error {
		return runGoreleaser(c, []string{"release"})
	},
}

var releaseBundleCmd = &cli.Command{
	Name:  "bundle",
	Usage: "Build release bundle (snapshot, no publish)",
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
	Action: func(c *cli.Context) error {
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

		goreleaserPath, err := EnsureToolBuilt(projectDir, toolsDir, "goreleaser", verbose)
		if err != nil {
			return err
		}

		// First run check
		checkCmd := exec.Command(goreleaserPath, "check")
		checkCmd.Dir = projectDir
		checkCmd.Stdout = os.Stdout
		checkCmd.Stderr = os.Stderr
		if err := checkCmd.Run(); err != nil {
			return err
		}

		// Then run release
		args := []string{"release", "--snapshot", "--clean", "--skip-publish"}
		args = append(args, c.Args().Slice()...)

		cmd := exec.Command(goreleaserPath, args...)
		cmd.Dir = projectDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	},
}

var releaseBuildCmd = &cli.Command{
	Name:  "build",
	Usage: "Build release binaries (single target, snapshot)",
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
	Action: func(c *cli.Context) error {
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

		goreleaserPath, err := EnsureToolBuilt(projectDir, toolsDir, "goreleaser", verbose)
		if err != nil {
			return err
		}

		// First run check
		checkCmd := exec.Command(goreleaserPath, "check")
		checkCmd.Dir = projectDir
		checkCmd.Stdout = os.Stdout
		checkCmd.Stderr = os.Stderr
		if err := checkCmd.Run(); err != nil {
			return err
		}

		// Then run build
		args := []string{"build", "--single-target", "--snapshot", "--clean"}
		args = append(args, c.Args().Slice()...)

		cmd := exec.Command(goreleaserPath, args...)
		cmd.Dir = projectDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	},
}

var releaseCheckCmd = &cli.Command{
	Name:  "check",
	Usage: "Check goreleaser configuration",
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
	Action: func(c *cli.Context) error {
		return runGoreleaser(c, []string{"check"})
	},
}

func runGoreleaser(c *cli.Context, args []string) error {
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

	goreleaserPath, err := EnsureToolBuilt(projectDir, toolsDir, "goreleaser", verbose)
	if err != nil {
		return err
	}

	args = append(args, c.Args().Slice()...)

	cmd := exec.Command(goreleaserPath, args...)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
