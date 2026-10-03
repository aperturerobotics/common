package main

import (
	"os"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/common/protogen"
	"github.com/pkg/errors"
)

// generateCmd generates selected protobuf bindings and reports Rust output drift.
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
		&cli.StringSliceFlag{
			Name:    "language",
			Aliases: []string{"l", "languages"},
			Usage:   "Output language to generate: go, ts, cpp, rust, csharp, python (can be specified multiple times)",
		},
		&cli.StringSliceFlag{
			Name:  "rpc",
			Usage: "RPC stub libraries to generate: starpc, starpc-python, none, false (can be specified multiple times)",
		},
		&cli.StringFlag{
			Name:    "project-dir",
			Aliases: []string{"C"},
			Usage:   "Project directory",
		},
		&cli.BoolFlag{
			Name:  "check",
			Usage: "Report outdated whole-graph Rust output without writing or preparing dependencies (requires aptre.rust)",
		},
		&cli.BoolFlag{
			Name:  "deps",
			Usage: "Ensure dependencies before generating",
			Value: true,
		},
	},
	Action: runGenerate,
}

// runGenerate resolves project configuration and dispatches each selected generation mode.
func runGenerate(c *cli.Context) error {
	// Explicit flags override the project's generation settings.
	cfg := protogen.NewConfig()
	cfg.Targets = c.StringSlice("targets")
	cfg.Exclude = c.StringSlice("exclude")
	cfg.Force = c.Bool("force")
	cfg.CacheFile = c.String("cache-file")
	cfg.Verbose = c.Bool("verbose")

	// Tool locations and Go features apply to both generation dispatch paths.
	cfg.GoLiteFeatures = c.String("features")
	cfg.ToolsDir = c.String("tools-dir")
	cfg.ProjectDir = c.String("project-dir")

	// Language and service flags override package defaults only when explicitly set.
	if c.IsSet("language") {
		cfg.Languages = c.StringSlice("language")
	}
	if c.IsSet("rpc") {
		cfg.RPCLibraries = c.StringSlice("rpc")
	}

	// Extra args are passed through
	cfg.ExtraArgs = c.Args().Slice()

	// A project that configures aptre.rust generates Rust from the whole schema
	// graph in one run; the per-directory generator handles the other languages.
	langs, err := cfg.GetLanguages()
	if err != nil {
		return err
	}
	rustConfig, err := cfg.GetRust()
	if err != nil {
		return err
	}
	wholeGraphRust := rustConfig != nil && langs.Has(protogen.LanguageRust)
	others := langs.Without(protogen.LanguageRust)

	// Read-only checking applies to the complete Rust output set alone.
	check := c.Bool("check")
	if check && (!wholeGraphRust || len(others) != 0) {
		return errors.New("--check requires aptre.rust and --language rust as the only language")
	}

	// Preparation can build executables and install packages, so a read-only
	// check never runs it; a missing plugin is reported by the generator.
	if c.Bool("deps") && !check {
		if err := ensureGenerateDeps(cfg, cfg.Verbose); err != nil {
			return errors.Wrap(err, "failed to ensure dependencies")
		}
	}

	// Generate Rust once over the whole graph before any per-directory work.
	if wholeGraphRust {
		if err := runRustGenerate(c, cfg, check); err != nil {
			return err
		}
		if len(others) == 0 {
			return nil
		}
		cfg.Languages = others.Names()
	}

	// Remaining languages retain their existing per-directory cache and output path.
	gen, err := protogen.NewGenerator(cfg)
	if err != nil {
		return errors.Wrap(err, "failed to create generator")
	}

	return gen.Generate(c.Context)
}

// runRustGenerate generates or checks the whole-graph Rust output.
func runRustGenerate(c *cli.Context, cfg *protogen.Config, check bool) error {
	// Resolve graph configuration and required plugins before touching outputs.
	gen, err := protogen.NewRustGenerator(cfg)
	if err != nil {
		return errors.Wrap(err, "failed to create rust generator")
	}

	// A normal run reports the files it actually wrote or removed.
	if !check {
		changed, err := gen.Generate(c.Context)
		if err != nil {
			return err
		}
		for _, file := range changed {
			if _, err := os.Stdout.WriteString("updated: " + file + "\n"); err != nil {
				return err
			}
		}
		return nil
	}

	// A check reports every difference without preparing dependencies or writing files.
	outdated, err := gen.Check(c.Context)
	if err != nil {
		return err
	}
	for _, file := range outdated {
		if _, err := os.Stdout.WriteString("outdated: " + file + "\n"); err != nil {
			return err
		}
	}
	if len(outdated) != 0 {
		return errors.Errorf("%d generated files are outdated; run aptre generate", len(outdated))
	}
	return nil
}

// cleanCmd removes the outputs recorded by the per-directory generator.
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

// runClean removes generated files through the project's existing manifest.
func runClean(c *cli.Context) error {
	// Select the configured project and manifest before constructing the generator.
	cfg := protogen.NewConfig()
	cfg.CacheFile = c.String("cache-file")
	cfg.ProjectDir = c.String("project-dir")

	// The existing generator owns which manifest entries can be removed.
	gen, err := protogen.NewGenerator(cfg)
	if err != nil {
		return errors.Wrap(err, "failed to create generator")
	}

	return gen.Clean()
}
