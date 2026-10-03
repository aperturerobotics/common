package protogen

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
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
	// ModuleDir is the resolved Go module root directory.
	ModuleDir string
	// ModulePath is the Go module path.
	ModulePath string
	// VendorDir is the vendor directory.
	VendorDir string
	// TsImportBoundaries are module-relative boundaries that trigger @go/ rewrites.
	TsImportBoundaries []string
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
	// Resolve the project whose schemas and package configuration will be read.
	projectDir, err := cfg.GetProjectDir()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get project directory")
	}

	// The containing Go module supplies vendored dependencies.
	moduleDir, err := cfg.GetModuleDir()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get module directory")
	}

	// Protoc imports name the project by its module-relative identity.
	modulePath, err := cfg.GetGoModule()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get Go module")
	}

	// Locate the manifest that records previous generated outputs.
	cacheFile, err := cfg.GetCacheFilePath()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get cache file path")
	}

	// Restore the prior generation state before selecting changed packages.
	cache, err := LoadCache(cacheFile)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load cache")
	}

	// Resolve the configured generators from the project's tools.
	plugins, err := DiscoverPlugins(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to discover plugins")
	}

	// TypeScript package boundaries determine generated import rewrites.
	tsImportBoundaries, err := cfg.GetTsImportBoundaries()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get ts import boundaries")
	}

	// Retain the resolved configuration and output streams for this run.
	vendorDir := filepath.Join(moduleDir, "vendor")
	outDir := vendorDir
	return &Generator{
		Config:             cfg,
		Plugins:            plugins,
		Cache:              cache,
		ProjectDir:         projectDir,
		ModuleDir:          moduleDir,
		ModulePath:         modulePath,
		VendorDir:          vendorDir,
		TsImportBoundaries: tsImportBoundaries,
		OutDir:             outDir,
		Verbose:            cfg.Verbose,
		Stdout:             os.Stdout,
		Stderr:             os.Stderr,
	}, nil
}

// Generate runs the proto generation.
func (g *Generator) Generate(ctx context.Context) error {
	// Protoc's canonical module paths resolve through temporary project symlinks.
	defer g.cleanupProjectSymlinks()
	if err := g.setupProjectSymlinks(); err != nil {
		return errors.Wrap(err, "failed to setup project symlinks")
	}

	// Discover proto files
	protoFiles, err := DiscoverProtoFiles(g.ProjectDir, g.Config.Targets, g.Config.Exclude)
	if err != nil {
		return errors.Wrap(err, "failed to discover proto files")
	}

	// Empty selection completes without invoking any generator.
	if len(protoFiles) == 0 {
		if g.Verbose {
			_, err := io.WriteString(g.Stdout, "No proto files found\n")
			return err
		}
		return nil
	}

	// Report the complete input count before filtering by the manifest cache.
	if g.Verbose {
		if _, err := io.WriteString(g.Stdout, "Found "+strconv.Itoa(len(protoFiles))+" proto files\n"); err != nil {
			return err
		}
	}

	// Get tool versions for cache invalidation.
	toolVersions := g.getToolVersions()

	// Build protoc arguments
	protocArgs := g.buildProtocArgs()
	flagsHash := HashProtocFlags(protocArgs, g.ModuleDir)

	// Group proto files by directory for cache tracking
	filesByDir := make(map[string][]string)
	for _, f := range protoFiles {
		dir := filepath.Dir(f)
		filesByDir[dir] = append(filesByDir[dir], f)
	}

	// Sort directories for deterministic processing order.
	dirs := make([]string, 0, len(filesByDir))
	for dir := range filesByDir {
		dirs = append(dirs, dir)
	}
	slices.Sort(dirs)

	// Track current packages and determine which need regeneration
	currentPackages := make(map[string]struct{})
	var filesToGenerate []string

	// Collect only packages whose source, flags or generator versions changed.
	for _, dir := range dirs {
		files := filesByDir[dir]
		packageKey := GetPackageKey(g.ModulePath, files[0])
		currentPackages[packageKey] = struct{}{}

		// Check if regeneration is needed
		needsRegen, err := g.Cache.NeedsRegeneration(packageKey, files, g.ProjectDir, flagsHash, toolVersions, g.Config.Force)
		if err != nil {
			return errors.Wrapf(err, "failed to check cache for %s", dir)
		}

		if !needsRegen {
			if g.Verbose {
				if _, err := io.WriteString(g.Stdout, "Skipping "+dir+" (up to date)\n"); err != nil {
					return err
				}
			}
			continue
		}

		if g.Verbose {
			if _, err := io.WriteString(g.Stdout, "Will generate "+dir+"\n"); err != nil {
				return err
			}
		}
		filesToGenerate = append(filesToGenerate, files...)
	}

	// Run protoc once for all files that need regeneration
	if len(filesToGenerate) > 0 {
		if g.Verbose {
			if _, err := io.WriteString(g.Stdout, "Generating "+strconv.Itoa(len(filesToGenerate))+" proto files\n"); err != nil {
				return err
			}
		}

		if err := g.runProtoc(ctx, filesToGenerate); err != nil {
			return errors.Wrap(err, "failed to generate protos")
		}

		// Post-process and update cache for each directory
		postProcessor := NewPostProcessor(
			g.ProjectDir,
			g.VendorDir,
			g.ModulePath,
			g.TsImportBoundaries,
			g.Verbose,
		)
		for _, dir := range dirs {
			files := filesByDir[dir]
			// Skip if not in files to generate
			shouldProcess := false
			for _, f := range files {
				if slices.Contains(filesToGenerate, f) {
					shouldProcess = true
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
					return errors.Wrapf(err, "failed to post-process %s", f)
				}
			}

			// Find generated files and update cache
			packageKey := GetPackageKey(g.ModulePath, files[0])
			var generatedFiles []string
			for _, f := range files {
				gf, err := FindGeneratedFilesForProto(f, g.ProjectDir, g.VendorDir, g.ModulePath, g.Plugins.Languages, g.Plugins.RPCLibraries)
				if err != nil {
					return errors.Wrapf(err, "failed to find generated files for %s", f)
				}
				generatedFiles = append(generatedFiles, gf...)
			}

			if previous := g.Cache.Packages[packageKey]; previous != nil {
				for _, old := range previous.GeneratedFiles {
					if slices.Contains(generatedFiles, old) {
						continue
					}
					if err := os.Remove(filepath.Join(g.ProjectDir, old)); err != nil && !os.IsNotExist(err) {
						return errors.Wrapf(err, "failed to remove stale generated file %s", old)
					}
				}
			}
			if err := g.Cache.UpdatePackage(packageKey, files, generatedFiles, g.ProjectDir); err != nil {
				return errors.Wrapf(err, "failed to update cache for %s", dir)
			}
		}
	}

	// Clean orphaned packages from cache
	g.Cache.CleanOrphanedPackages(currentPackages)

	// Persist the flags and tool identity used to select this output set.
	g.Cache.SetProtocFlags(protocArgs, g.ModuleDir)
	g.Cache.SetToolVersions(toolVersions)
	cacheFile, err := g.Config.GetCacheFilePath()
	if err != nil {
		return err
	}
	if err := g.Cache.Save(cacheFile); err != nil {
		return errors.Wrap(err, "failed to save cache")
	}

	// Format generated files
	if len(filesToGenerate) > 0 {
		if err := g.formatGeneratedFiles(filesToGenerate); err != nil {
			return errors.Wrap(err, "failed to format generated files")
		}
	}

	return nil
}

// setupProjectSymlinks maps protoc's native and Python module paths to the project.
func (g *Generator) setupProjectSymlinks() error {
	for _, modulePath := range []string{
		g.ModulePath,
		pythonModulePath(g.ModulePath),
	} {
		symlinkPath := filepath.Join(g.VendorDir, modulePath)
		if err := os.MkdirAll(filepath.Dir(symlinkPath), 0o755); err != nil {
			return err
		}
		if err := os.Remove(symlinkPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.Symlink(g.ProjectDir, symlinkPath); err != nil {
			return err
		}
	}
	return nil
}

// cleanupProjectSymlinks removes the temporary protoc output mappings.
func (g *Generator) cleanupProjectSymlinks() {
	for _, modulePath := range []string{
		g.ModulePath,
		pythonModulePath(g.ModulePath),
	} {
		_ = os.Remove(filepath.Join(g.VendorDir, modulePath))
	}
}

// pythonModulePath matches protoc's Python filename normalization.
func pythonModulePath(modulePath string) string {
	modulePath = strings.ReplaceAll(modulePath, ".", string(filepath.Separator))
	return strings.ReplaceAll(modulePath, "-", "_")
}

// buildProtocArgs builds the protoc command arguments.
func (g *Generator) buildProtocArgs() []string {
	// Include paths
	var args []string
	args = append(args, "-I", g.OutDir)
	args = append(args, "--proto_path", g.OutDir)

	// Add include path for google well-known proto types (timestamp.proto, any.proto, etc.)
	// These are located at vendor/github.com/aperturerobotics/protobuf/src
	protobufSrcDir := filepath.Join(g.VendorDir, "github.com", "aperturerobotics", "protobuf", "src")
	if _, err := os.Stat(protobufSrcDir); err == nil {
		args = append(args, "-I", protobufSrcDir)
	}

	// Output and plugin arguments
	args = append(args, g.Plugins.GetProtocArgs(g.OutDir, g.ProjectDir)...)

	// Extra arguments from config
	args = append(args, g.Config.ExtraArgs...)

	return args
}

// runProtoc runs protoc for the given proto files using go-protoc-wasi.
func (g *Generator) runProtoc(ctx context.Context, protoFiles []string) error {
	// Build arguments with the vendor prefix that names each proto file.
	args := g.buildProtocArgs()
	for _, f := range protoFiles {
		args = append(args, filepath.Join(g.VendorDir, g.ModulePath, f))
	}

	// The vendor directory supplies imports and receives the output.
	run := &ProtocRun{
		Plugins: g.Plugins,
		Mounts:  []string{g.VendorDir, g.ProjectDir},
		Args:    args,
		Verbose: g.Verbose,
		Stdout:  g.Stdout,
	}
	return run.Run(ctx)
}

// getToolVersions returns a string with tool versions for cache invalidation.
func (g *Generator) getToolVersions() string {
	// Get protoc version (embedded in go-protoc-wasi)
	var versions []string
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
					if before, _, ok := strings.Cut(rest, `"`); ok {
						versions = append(versions, "protobuf-es-lite="+before)
					}
				}
			}
		}
	}

	// Python's lockfile identity invalidates output after environment changes.
	if g.Plugins != nil && g.Plugins.StarpcPython != nil {
		if data, err := os.ReadFile(filepath.Join(g.ProjectDir, "uv.lock")); err == nil {
			digest := sha256.Sum256(data)
			versions = append(versions, "uv.lock="+hex.EncodeToString(digest[:]))
		}
	}

	return strings.Join(versions, ",")
}

// formatGeneratedFiles formats the generated Go and TypeScript files.
func (g *Generator) formatGeneratedFiles(protoFiles []string) error {
	// Select only files produced for the requested schemas and enabled languages.
	var goFiles, tsFiles []string
	for _, f := range protoFiles {
		gf, err := FindGeneratedFilesForProto(f, g.ProjectDir, g.VendorDir, g.ModulePath, g.Plugins.Languages, g.Plugins.RPCLibraries)
		if err != nil {
			continue
		}
		for _, genFile := range gf {
			switch {
			case strings.HasSuffix(genFile, ".pb.go"):
				goFiles = append(goFiles, genFile)
			case strings.HasSuffix(genFile, ".pb.ts"):
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
			for attempt := range 3 {
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
					// Preserve the formatter failure if its diagnostic output also fails.
					_, _ = io.WriteString(g.Stderr, errOutput)
					return errors.Wrap(lastErr, "gofumpt failed")
				}
				// It's the race condition error, retry
				if g.Verbose {
					if _, err := io.WriteString(g.Stdout, "gofumpt race condition detected, retrying (attempt "+strconv.Itoa(attempt+1)+"/3)...\n"); err != nil {
						return err
					}
				}
			}
			if lastErr != nil {
				return errors.Wrap(lastErr, "gofumpt failed after retries")
			}
		}
	}

	// Format TypeScript files with oxfmt
	if len(tsFiles) > 0 {
		oxfmtConfig := filepath.Join(g.ProjectDir, g.Config.ToolsDir, ".oxfmtrc.json")
		if _, err := os.Stat(oxfmtConfig); err == nil {
			args := []string{"-c", oxfmtConfig}
			args = append(args, tsFiles...)
			cmd := exec.Command("oxfmt", args...)
			cmd.Dir = g.ProjectDir
			cmd.Stdout = g.Stdout
			cmd.Stderr = g.Stderr
			_ = cmd.Run() // Ignore oxfmt errors
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
