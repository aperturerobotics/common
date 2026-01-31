package main

import (
	"fmt"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/common/protogen"
)

var generateCmd = &cli.Command{
	Name:    "generate",
	Aliases: []string{"gen", "genproto"},
	Usage:   "Generate protobuf code",
	Flags: []cli.Flag{
		&cli.StringSliceFlag{
			Name:    "targets",
			Aliases: []string{"t"},
			Usage:   "Proto file patterns (can be specified multiple times)",
			Value:   cli.NewStringSlice("./*.proto"),
		},
		&cli.StringSliceFlag{
			Name:    "exclude",
			Aliases: []string{"e"},
			Usage:   "Proto file patterns to exclude (can be specified multiple times)",
		},
		&cli.BoolFlag{
			Name:    "force",
			Aliases: []string{"f"},
			Usage:   "Regenerate all files regardless of cache",
		},
		&cli.StringFlag{
			Name:  "cache-file",
			Usage: "Path to the cache file",
			Value: protogen.DefaultCacheFile,
		},
		&cli.BoolFlag{
			Name:    "verbose",
			Aliases: []string{"v"},
			Usage:   "Enable verbose output",
		},
		&cli.StringFlag{
			Name:  "features",
			Usage: "Go-lite features to enable",
			Value: protogen.DefaultGoLiteFeatures,
		},
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
			Name:  "deps",
			Usage: "Ensure dependencies before generating",
			Value: true,
		},
	},
	Action: runGenerate,
}

func runGenerate(c *cli.Context) error {
	cfg := protogen.NewConfig()
	cfg.Targets = c.StringSlice("targets")
	cfg.Exclude = c.StringSlice("exclude")
	cfg.Force = c.Bool("force")
	cfg.CacheFile = c.String("cache-file")
	cfg.Verbose = c.Bool("verbose")
	cfg.GoLiteFeatures = c.String("features")
	cfg.ToolsDir = c.String("tools-dir")
	cfg.ProjectDir = c.String("project-dir")

	// Extra args are passed through
	cfg.ExtraArgs = c.Args().Slice()

	// Ensure dependencies if requested
	if c.Bool("deps") {
		if err := ensureDeps(cfg.ProjectDir, cfg.ToolsDir, cfg.Verbose); err != nil {
			return fmt.Errorf("failed to ensure dependencies: %w", err)
		}
	}

	gen, err := protogen.NewGenerator(cfg)
	if err != nil {
		return fmt.Errorf("failed to create generator: %w", err)
	}

	return gen.Generate(c.Context)
}

var cleanCmd = &cli.Command{
	Name:  "clean",
	Usage: "Remove generated files and cache",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "cache-file",
			Usage: "Path to the cache file",
			Value: protogen.DefaultCacheFile,
		},
		&cli.StringFlag{
			Name:    "project-dir",
			Aliases: []string{"C"},
			Usage:   "Project directory",
		},
	},
	Action: runClean,
}

func runClean(c *cli.Context) error {
	cfg := protogen.NewConfig()
	cfg.CacheFile = c.String("cache-file")
	cfg.ProjectDir = c.String("project-dir")

	gen, err := protogen.NewGenerator(cfg)
	if err != nil {
		return fmt.Errorf("failed to create generator: %w", err)
	}

	return gen.Clean()
}
