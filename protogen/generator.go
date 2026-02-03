package protogen

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	protoc "github.com/aperturerobotics/go-protoc-wasi"
	"github.com/tetratelabs/wazero"
)

// Generator handles protobuf code generation.
type Generator struct {
	// Config is the generator configuration.
	Config *Config
	// Plugins contains the discovered plugins.
	Plugins *Plugins
	// Cache is the manifest cache.
	Cache *Cache
	// ProjectDir is the resolved project directory.
	ProjectDir string
	// ModulePath is the Go module path.
	ModulePath string
	// VendorDir is the vendor directory.
	VendorDir string
	// OutDir is the output directory (same as VendorDir).
	OutDir string
	// Verbose enables verbose output.
	Verbose bool
	// Stdout is where to write standard output.
	Stdout io.Writer
	// Stderr is where to write error output.
	Stderr io.Writer
}

// NewGenerator creates a new Generator.
func NewGenerator(cfg *Config) (*Generator, error) {
	projectDir, err := cfg.GetProjectDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get project directory: %w", err)
	}

	modulePath, err := cfg.GetGoModule()
	if err != nil {
		return nil, fmt.Errorf("failed to get Go module: %w", err)
	}

	cacheFile, err := cfg.GetCacheFilePath()
	if err != nil {
		return nil, fmt.Errorf("failed to get cache file path: %w", err)
	}

	cache, err := LoadCache(cacheFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load cache: %w", err)
	}

	plugins, err := DiscoverPlugins(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to discover plugins: %w", err)
	}

	vendorDir := filepath.Join(projectDir, "vendor")
	outDir := vendorDir

	return &Generator{
		Config:     cfg,
		Plugins:    plugins,
		Cache:      cache,
		ProjectDir: projectDir,
		ModulePath: modulePath,
		VendorDir:  vendorDir,
		OutDir:     outDir,
		Verbose:    cfg.Verbose,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
	}, nil
}

// Generate runs the proto generation.
func (g *Generator) Generate(ctx context.Context) error {
	// Set up vendor symlink
	if err := g.setupVendorSymlink(); err != nil {
		return fmt.Errorf("failed to setup vendor symlink: %w", err)
	}
	defer g.cleanupVendorSymlink()

	// Discover proto files
	protoFiles, err := DiscoverProtoFiles(g.ProjectDir, g.Config.Targets, g.Config.Exclude)
	if err != nil {
		return fmt.Errorf("failed to discover proto files: %w", err)
	}

	if len(protoFiles) == 0 {
		if g.Verbose {
			fmt.Fprintln(g.Stdout, "No proto files found")
		}
		return nil
	}

	if g.Verbose {
		fmt.Fprintf(g.Stdout, "Found %d proto files\n", len(protoFiles))
	}

	// Get tool versions for cache invalidation
	toolVersions := g.getToolVersions()
	g.Cache.SetToolVersions(toolVersions)

	// Build protoc arguments
	protocArgs := g.buildProtocArgs()
	g.Cache.SetProtocFlags(protocArgs)
	flagsHash := hashStrings(protocArgs)

	// Group proto files by directory for cache tracking
	filesByDir := make(map[string][]string)
	for _, f := range protoFiles {
		dir := filepath.Dir(f)
		filesByDir[dir] = append(filesByDir[dir], f)
	}

	// Track current packages and determine which need regeneration
	currentPackages := make(map[string]struct{})
	var filesToGenerate []string

	for dir, files := range filesByDir {
		packageKey := GetPackageKey(g.ModulePath, files[0])
		currentPackages[packageKey] = struct{}{}

		// Check if regeneration is needed
		needsRegen, err := g.Cache.NeedsRegeneration(packageKey, files, g.ProjectDir, flagsHash, g.Config.Force)
		if err != nil {
			return fmt.Errorf("failed to check cache for %s: %w", dir, err)
		}

		if !needsRegen {
			if g.Verbose {
				fmt.Fprintf(g.Stdout, "Skipping %s (up to date)\n", dir)
			}
			continue
		}

		if g.Verbose {
			fmt.Fprintf(g.Stdout, "Will generate %s\n", dir)
		}
		filesToGenerate = append(filesToGenerate, files...)
	}

	// Run protoc once for all files that need regeneration
	if len(filesToGenerate) > 0 {
		if g.Verbose {
			fmt.Fprintf(g.Stdout, "Generating %d proto files\n", len(filesToGenerate))
		}

		if err := g.runProtoc(ctx, filesToGenerate); err != nil {
			return fmt.Errorf("failed to generate protos: %w", err)
		}

		// Post-process and update cache for each directory
		postProcessor := NewPostProcessor(g.ProjectDir, g.ModulePath, g.Verbose)
		for dir, files := range filesByDir {
			// Skip if not in files to generate
			shouldProcess := false
			for _, f := range files {
				for _, fg := range filesToGenerate {
					if f == fg {
						shouldProcess = true
						break
					}
				}
				if shouldProcess {
					break
				}
			}
			if !shouldProcess {
				continue
			}

			// Post-process generated files
			for _, f := range files {
				if err := postProcessor.ProcessGeneratedFiles(f); err != nil {
					return fmt.Errorf("failed to post-process %s: %w", f, err)
				}
			}

			// Find generated files and update cache
			packageKey := GetPackageKey(g.ModulePath, files[0])
			var generatedFiles []string
			for _, f := range files {
				gf, err := FindGeneratedFilesForProto(f, g.ProjectDir, g.ModulePath)
				if err != nil {
					return fmt.Errorf("failed to find generated files for %s: %w", f, err)
				}
				generatedFiles = append(generatedFiles, gf...)
			}

			if err := g.Cache.UpdatePackage(packageKey, files, generatedFiles, g.ProjectDir); err != nil {
				return fmt.Errorf("failed to update cache for %s: %w", dir, err)
			}
		}
	}

	// Clean orphaned packages from cache
	g.Cache.CleanOrphanedPackages(currentPackages)

	// Save cache
	cacheFile, _ := g.Config.GetCacheFilePath()
	if err := g.Cache.Save(cacheFile); err != nil {
		return fmt.Errorf("failed to save cache: %w", err)
	}

	// Format generated files
	if len(filesToGenerate) > 0 {
		if err := g.formatGeneratedFiles(filesToGenerate); err != nil {
			return fmt.Errorf("failed to format generated files: %w", err)
		}
	}

	return nil
}

// setupVendorSymlink creates a symlink from vendor/MODULE to the project dir.
func (g *Generator) setupVendorSymlink() error {
	// Ensure vendor directory exists
	vendorModuleDir := filepath.Join(g.VendorDir, filepath.Dir(g.ModulePath))
	if err := os.MkdirAll(vendorModuleDir, 0o755); err != nil {
		return err
	}

	// Remove existing symlink
	symlinkPath := filepath.Join(g.VendorDir, g.ModulePath)
	_ = os.Remove(symlinkPath)

	// Create symlink
	return os.Symlink(g.ProjectDir, symlinkPath)
}

// cleanupVendorSymlink removes the vendor symlink.
func (g *Generator) cleanupVendorSymlink() {
	symlinkPath := filepath.Join(g.VendorDir, g.ModulePath)
	_ = os.Remove(symlinkPath)
}

// buildProtocArgs builds the protoc command arguments.
func (g *Generator) buildProtocArgs() []string {
	var args []string

	// Include paths
	args = append(args, "-I", g.OutDir)
	args = append(args, "--proto_path", g.OutDir)

	// Add include path for google well-known proto types (timestamp.proto, any.proto, etc.)
	// These are located at vendor/github.com/aperturerobotics/protobuf/src
	protobufSrcDir := filepath.Join(g.VendorDir, "github.com", "aperturerobotics", "protobuf", "src")
	if _, err := os.Stat(protobufSrcDir); err == nil {
		args = append(args, "-I", protobufSrcDir)
	}

	// Plugin arguments
	args = append(args, g.Plugins.GetProtocArgs(g.OutDir)...)

	// Extra arguments from config
	args = append(args, g.Config.ExtraArgs...)

	return args
}

// runProtoc runs protoc for the given proto files using go-protoc-wasi.
func (g *Generator) runProtoc(ctx context.Context, protoFiles []string) error {
	var stdout, stderr bytes.Buffer

	// Create wazero runtime
	runtime := wazero.NewRuntime(ctx)
	defer runtime.Close(ctx)

	// Create plugin handler
	pluginHandler := NewNativePluginHandler(g.Plugins, g.Verbose)

	// Create filesystem config that mounts the vendor directory
	// This allows protoc to read .proto files and write output files
	fsConfig := wazero.NewFSConfig().
		WithDirMount(g.VendorDir, g.VendorDir).
		WithDirMount(g.ProjectDir, g.ProjectDir)

	// Create protoc config
	cfg := &protoc.Config{
		Stdout:        &stdout,
		Stderr:        &stderr,
		FSConfig:      fsConfig,
		PluginHandler: pluginHandler,
	}

	// Create protoc instance
	p, err := protoc.NewProtoc(ctx, runtime, cfg)
	if err != nil {
		return fmt.Errorf("failed to create protoc: %w", err)
	}
	defer p.Close(ctx)

	// Initialize protoc (this instantiates WASI)
	if err := p.Init(ctx); err != nil {
		return fmt.Errorf("failed to init protoc: %w", err)
	}

	// Initialize prost WASM plugin if rust prost is configured
	// This must be done after protoc.Init since that's when WASI gets instantiated
	if g.Plugins.RustProst != nil {
		if err := pluginHandler.InitProstWASM(ctx, runtime); err != nil {
			return fmt.Errorf("failed to init prost WASM: %w", err)
		}
		defer pluginHandler.CloseProstWASM(ctx)
	}

	// Build arguments
	args := []string{"protoc"}
	args = append(args, g.buildProtocArgs()...)

	// Add proto files with vendor prefix
	for _, f := range protoFiles {
		args = append(args, filepath.Join(g.VendorDir, g.ModulePath, f))
	}

	if g.Verbose {
		fmt.Fprintf(g.Stdout, "Running: %s\n", strings.Join(args, " "))
	}

	// Run protoc
	exitCode, err := p.Run(ctx, args)
	if err != nil {
		return fmt.Errorf("protoc error: %w", err)
	}

	if exitCode != 0 {
		if stderr.Len() > 0 {
			return fmt.Errorf("protoc failed with exit code %d: %s", exitCode, stderr.String())
		}
		return fmt.Errorf("protoc failed with exit code %d", exitCode)
	}

	if g.Verbose && stdout.Len() > 0 {
		fmt.Fprint(g.Stdout, stdout.String())
	}

	return nil
}

// getToolVersions returns a string with tool versions for cache invalidation.
func (g *Generator) getToolVersions() string {
	var versions []string

	// Get protoc version (embedded in go-protoc-wasi)
	versions = append(versions, "protoc=embedded")

	// Get Go tool versions from tools/go.mod
	toolsGoMod := filepath.Join(g.ProjectDir, g.Config.ToolsDir, "go.mod")
	if data, err := os.ReadFile(toolsGoMod); err == nil {
		scanner := bufio.NewScanner(bytes.NewReader(data))
		for scanner.Scan() {
			line := scanner.Text()
			// Skip replace directives
			if strings.Contains(line, "=>") {
				continue
			}
			if strings.Contains(line, "github.com/aperturerobotics/protobuf-go-lite") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					versions = append(versions, "protobuf-go-lite="+parts[1])
				}
			}
			if strings.Contains(line, "github.com/aperturerobotics/starpc") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					versions = append(versions, "starpc="+parts[1])
				}
			}
		}
	}

	// Get TypeScript tool versions from package.json
	packageJSON := filepath.Join(g.ProjectDir, "package.json")
	if data, err := os.ReadFile(packageJSON); err == nil {
		content := string(data)
		if idx := strings.Index(content, "@aptre/protobuf-es-lite"); idx >= 0 {
			// Extract version (simplistic parsing)
			rest := content[idx:]
			if vStart := strings.Index(rest, ":"); vStart >= 0 {
				rest = rest[vStart+1:]
				if vStart := strings.Index(rest, `"`); vStart >= 0 {
					rest = rest[vStart+1:]
					if vEnd := strings.Index(rest, `"`); vEnd >= 0 {
						versions = append(versions, "protobuf-es-lite="+rest[:vEnd])
					}
				}
			}
		}
	}

	return strings.Join(versions, ",")
}

// formatGeneratedFiles formats the generated Go and TypeScript files.
func (g *Generator) formatGeneratedFiles(protoFiles []string) error {
	var goFiles, tsFiles []string

	for _, f := range protoFiles {
		gf, err := FindGeneratedFilesForProto(f, g.ProjectDir, g.ModulePath)
		if err != nil {
			continue
		}
		for _, genFile := range gf {
			if strings.HasSuffix(genFile, ".pb.go") {
				goFiles = append(goFiles, genFile)
			} else if strings.HasSuffix(genFile, ".pb.ts") {
				tsFiles = append(tsFiles, genFile)
			}
		}
	}

	// Format Go files with gofumpt
	if len(goFiles) > 0 {
		gofumptPath := filepath.Join(g.ProjectDir, g.Config.ToolsDir, "bin", "gofumpt")
		if _, err := os.Stat(gofumptPath); err == nil {
			// Retry gofumpt up to 3 times with a small delay.
			// This works around a race condition where gofumpt may see a file size
			// mismatch if the file is still being flushed to disk after protoc writes.
			var lastErr error
			for attempt := 0; attempt < 3; attempt++ {
				if attempt > 0 {
					time.Sleep(100 * time.Millisecond)
				}
				args := append([]string{"-w"}, goFiles...)
				cmd := exec.Command(gofumptPath, args...)
				cmd.Dir = g.ProjectDir
				// Capture stderr to check for the specific race condition error
				var stderr bytes.Buffer
				cmd.Stdout = g.Stdout
				cmd.Stderr = &stderr
				lastErr = cmd.Run()
				if lastErr == nil {
					break
				}
				// Check if this is the "size changed during reading" error
				errOutput := stderr.String()
				if !strings.Contains(errOutput, "changed during reading") {
					// Different error, output it and fail
					fmt.Fprint(g.Stderr, errOutput)
					return fmt.Errorf("gofumpt failed: %w", lastErr)
				}
				// It's the race condition error, retry
				if g.Verbose {
					fmt.Fprintf(g.Stdout, "gofumpt race condition detected, retrying (attempt %d/3)...\n", attempt+1)
				}
			}
			if lastErr != nil {
				return fmt.Errorf("gofumpt failed after retries: %w", lastErr)
			}
		}
	}

	// Format TypeScript files with prettier
	if len(tsFiles) > 0 {
		prettierConfig := filepath.Join(g.ProjectDir, g.Config.ToolsDir, ".prettierrc.yaml")
		if _, err := os.Stat(prettierConfig); err == nil {
			args := []string{"--config", prettierConfig, "-w"}
			args = append(args, tsFiles...)
			cmd := exec.Command("prettier", args...)
			cmd.Dir = g.ProjectDir
			cmd.Stdout = g.Stdout
			cmd.Stderr = g.Stderr
			_ = cmd.Run() // Ignore prettier errors
		}
	}

	return nil
}

// Clean removes all generated files and the cache.
func (g *Generator) Clean() error {
	// Remove cache file
	cacheFile, err := g.Config.GetCacheFilePath()
	if err != nil {
		return err
	}
	_ = os.Remove(cacheFile)

	// Remove generated files listed in cache
	for _, pkg := range g.Cache.Packages {
		for _, f := range pkg.GeneratedFiles {
			fullPath := filepath.Join(g.ProjectDir, f)
			_ = os.Remove(fullPath)
		}
	}

	return nil
}
