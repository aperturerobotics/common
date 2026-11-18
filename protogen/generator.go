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
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
)

// Generator handles protobuf code generation.
type Generator struct {
	// Config is the generator configuration.
	Config *Config
	// Plugins contains the discovered plugins.
	Plugins *Plugins
	// ProjectDir is the resolved project directory.
	ProjectDir string
	// ModuleDir is the resolved Go module root directory.
	ModuleDir string
	// ModulePath is the Go module path.
	ModulePath string
	// VendorDir is the vendor directory, which supplies imported schemas.
	VendorDir string
	// TsImportBoundaries are module-relative boundaries that trigger @go/ rewrites.
	TsImportBoundaries []string
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
	return &Generator{
		Config:             cfg,
		Plugins:            plugins,
		ProjectDir:         projectDir,
		ModuleDir:          moduleDir,
		ModulePath:         modulePath,
		VendorDir:          filepath.Join(moduleDir, "vendor"),
		TsImportBoundaries: tsImportBoundaries,
		Verbose:            cfg.Verbose,
		Stdout:             os.Stdout,
		Stderr:             os.Stderr,
	}, nil
}

// Generate runs the proto generation.
func (g *Generator) Generate(ctx context.Context) error {
	// Protoc names the project's schemas and output by the module path, which
	// resolves through this run's private root.
	root, err := g.linkProtocRoot()
	if err != nil {
		return errors.Wrap(err, "failed to link protoc root")
	}
	defer os.RemoveAll(root)

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

	// Identify the protoc flags apart from the run's private root.
	flagsHash := HashProtocFlags(g.buildProtocArgs(protocRootIdentity), g.ModuleDir)

	// Restore only this run's package inventory while retaining the other runs.
	cacheFile, err := g.Config.GetCacheFilePath()
	if err != nil {
		return errors.Wrap(err, "failed to get cache file path")
	}
	manifest, err := LoadCache(cacheFile)
	if err != nil {
		return errors.Wrap(err, "failed to load cache")
	}
	runKey := g.runKey()
	run := manifest.Runs[runKey]
	if run == nil || run.Packages == nil {
		run = NewRunCache()
	}

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
		needsRegen, err := run.NeedsRegeneration(packageKey, files, g.ProjectDir, flagsHash, toolVersions, g.Config.Force)
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

		if err := g.runProtoc(ctx, root, filesToGenerate); err != nil {
			return errors.Wrap(err, "failed to generate protos")
		}

		// Index selected inputs once before visiting their directory groups.
		selectedFiles := make(map[string]struct{}, len(filesToGenerate))
		for _, file := range filesToGenerate {
			selectedFiles[file] = struct{}{}
		}

		// Post-process and update cache for each directory
		postProcessor := NewPostProcessor(
			g.ProjectDir,
			g.VendorDir,
			g.ModulePath,
			g.TsImportBoundaries,
			g.Verbose,
		)
		postProcessor.ProtocRoot = root
		for _, dir := range dirs {
			files := filesByDir[dir]
			// Skip if not in files to generate
			shouldProcess := false
			for _, f := range files {
				if _, ok := selectedFiles[f]; ok {
					shouldProcess = true
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
				gf, err := FindGeneratedFilesForProto(f, g.ProjectDir, g.Plugins.Languages, g.Plugins.RPCLibraries)
				if err != nil {
					return errors.Wrapf(err, "failed to find generated files for %s", f)
				}
				generatedFiles = append(generatedFiles, gf...)
			}

			// Compare prior outputs against indexed current names before removing any.
			if previous := run.Packages[packageKey]; previous != nil {
				currentOutputs := make(map[string]struct{}, len(generatedFiles))
				for _, file := range generatedFiles {
					currentOutputs[file] = struct{}{}
				}
				for _, old := range previous.GeneratedFiles {
					if _, ok := currentOutputs[old]; ok {
						continue
					}
					if err := os.Remove(filepath.Join(g.ProjectDir, old)); err != nil && !os.IsNotExist(err) {
						return errors.Wrapf(err, "failed to remove stale generated file %s", old)
					}
				}
			}
			if err := run.UpdatePackage(packageKey, files, generatedFiles, g.ProjectDir); err != nil {
				return errors.Wrapf(err, "failed to update cache for %s", dir)
			}
		}
	}

	// Finish writing generated files before marking them current.
	if len(filesToGenerate) > 0 {
		if err := g.formatGeneratedFiles(filesToGenerate); err != nil {
			return errors.Wrap(err, "failed to format generated files")
		}
	}

	// Prune only this inventory. Entries outside the current selection are
	// reusable only if their sources still exist and their tool identity matches.
	if run.ProtocFlagsHash != flagsHash || run.ToolVersions != toolVersions {
		run.DropOutsidePackages(currentPackages)
	} else {
		run.CleanOrphanedPackages(currentPackages, g.ProjectDir)
	}

	// Replace this run in the manifest without changing the others.
	run.ProtocFlagsHash = flagsHash
	run.ToolVersions = toolVersions
	manifest.Runs[runKey] = run
	if err := manifest.Save(cacheFile); err != nil {
		return errors.Wrap(err, "failed to save cache")
	}

	// Make a cache hit visible without requiring verbose per-package logs.
	if len(filesToGenerate) == 0 {
		_, err := io.WriteString(g.Stdout, "Generated protobuf files are up to date.\n")
		return err
	}
	return nil
}

// runKey identifies a language and source selection. Protoc options, including
// RPC plugins, invalidate it in place so it can remove obsolete outputs.
func (g *Generator) runKey() string {
	return HashProtocFlags([]string{
		g.ModulePath,
		relativizePath(g.ProjectDir, g.ModuleDir),
		strings.Join(g.Plugins.Languages.Names(), ","),
		HashProtocFlags(g.Config.Targets, ""),
		HashProtocFlags(g.Config.Exclude, ""),
	}, "")
}

// protocRootIdentity stands for the private protoc root in the flags hash, so
// the cache identity does not change with the root's temporary path.
const protocRootIdentity = "<protoc-root>"

// linkProtocRoot creates the private protoc root of one run: a directory
// holding only links from the module's Go and Python import paths to the
// project. Protoc resolves the project's schemas through it and writes its
// output into it, so the output lands in the project. The root is separate from
// vendor/, which may hold real directories under the module path, such as a
// vendored nested module of the project. The caller removes the root with
// os.RemoveAll, which removes the links and not their targets.
func (g *Generator) linkProtocRoot() (string, error) {
	// Create the root, removing it again if a link fails.
	root, err := os.MkdirTemp("", "aptre-protoc-")
	if err != nil {
		return "", err
	}
	for _, modulePath := range slices.Compact([]string{g.ModulePath, pythonModulePath(g.ModulePath)}) {
		link := filepath.Join(root, modulePath)
		if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
			_ = os.RemoveAll(root)
			return "", err
		}
		if err := os.Symlink(g.ProjectDir, link); err != nil {
			_ = os.RemoveAll(root)
			return "", err
		}
	}
	return root, nil
}

// pythonModulePath matches protoc's Python filename normalization.
func pythonModulePath(modulePath string) string {
	modulePath = strings.ReplaceAll(modulePath, ".", string(filepath.Separator))
	return strings.ReplaceAll(modulePath, "-", "_")
}

// buildProtocArgs builds the protoc arguments for a protoc root. The root is
// searched first, so the project's own schemas shadow any vendored copy.
func (g *Generator) buildProtocArgs(root string) []string {
	// Include paths
	var args []string
	args = append(args, "-I", root)
	if _, err := os.Stat(g.VendorDir); err == nil {
		args = append(args, "-I", g.VendorDir)
	}

	// Add include path for google well-known proto types (timestamp.proto, any.proto, etc.)
	// These are located at vendor/github.com/aperturerobotics/protobuf/src
	protobufSrcDir := filepath.Join(g.VendorDir, "github.com", "aperturerobotics", "protobuf", "src")
	if _, err := os.Stat(protobufSrcDir); err == nil {
		args = append(args, "-I", protobufSrcDir)
	}

	// Output and plugin arguments
	args = append(args, g.Plugins.GetProtocArgs(root, g.ProjectDir)...)

	// Extra arguments from config
	args = append(args, g.Config.ExtraArgs...)

	return args
}

// runProtoc runs protoc for the given proto files using go-protoc-wasi.
func (g *Generator) runProtoc(ctx context.Context, root string, protoFiles []string) error {
	// Name each proto file by its module path under the root.
	args := g.buildProtocArgs(root)
	for _, f := range protoFiles {
		args = append(args, filepath.Join(root, g.ModulePath, f))
	}

	// The root names the project and receives the output; vendor/, when the
	// project has one, supplies imports.
	mounts := []string{root, g.ProjectDir}
	if _, err := os.Stat(g.VendorDir); err == nil {
		mounts = append(mounts, g.VendorDir)
	}
	run := &ProtocRun{
		Plugins: g.Plugins,
		Mounts:  mounts,
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
	versions = append(versions, "protoc=embedded", "generator="+generatorIdentity())

	// Get Go tool versions from tools/go.mod
	toolsGoMod := filepath.Join(ToolsPath(g.ProjectDir, g.Config.ToolsDir), "go.mod")
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

// commonModulePath is the module that holds this generator.
const commonModulePath = "github.com/aperturerobotics/common"

// generatorIdentity names the source of the running generator, so a change to
// the generator regenerates every package even when no tool version moved.
// aptre's main package lives in common, so the build info names common as the
// main module even when a project runs it from its own pinned version. A
// released or pseudo-versioned common reports the module's go.sum hash, the
// same on every machine; a build inside common reports its clean VCS revision.
// A local replace or a modified tree has neither, so it reports a hash of the
// executable, which follows every edit.
var generatorIdentity = sync.OnceValue(func() string {
	// Name common by its checksum when the build resolved it as a module.
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Path != commonModulePath {
		return executableHash()
	}
	mod := &info.Main
	if mod.Replace != nil {
		mod = mod.Replace
	}
	if mod.Sum != "" {
		return mod.Sum
	}

	// Name common by its clean revision when built from its own tree.
	var revision string
	modified := true
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value != "false"
		}
	}
	if revision != "" && !modified {
		return "vcs:" + revision
	}

	// Otherwise hash the executable that holds the generator.
	return executableHash()
})

// executableHash returns the hex SHA-256 of the running executable, or
// "unknown" when it cannot be read.
func executableHash() string {
	// Open the running executable.
	path, err := os.Executable()
	if err != nil {
		return "unknown"
	}
	f, err := os.Open(path)
	if err != nil {
		return "unknown"
	}
	defer f.Close()

	// Hash its bytes.
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "unknown"
	}
	return "exe:" + hex.EncodeToString(h.Sum(nil))
}

// formatGeneratedFiles formats the generated Go and TypeScript files.
func (g *Generator) formatGeneratedFiles(protoFiles []string) error {
	// Select only files produced for the requested schemas and enabled languages.
	var goFiles, tsFiles []string
	for _, f := range protoFiles {
		gf, err := FindGeneratedFilesForProto(f, g.ProjectDir, g.Plugins.Languages, g.Plugins.RPCLibraries)
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
		gofumptPath := filepath.Join(ToolsPath(g.ProjectDir, g.Config.ToolsDir), "bin", "gofumpt")
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
		oxfmtConfig := filepath.Join(ToolsPath(g.ProjectDir, g.Config.ToolsDir), ".oxfmtrc.json")
		if _, err := os.Stat(oxfmtConfig); err == nil {
			args := []string{"-c", oxfmtConfig}
			args = append(args, tsFiles...)
			cmd := exec.Command(findOxfmt(g.ProjectDir), args...) //nolint:gosec // oxfmt and its arguments come from the project being generated.
			cmd.Dir = g.ProjectDir
			cmd.Stdout = g.Stdout
			cmd.Stderr = g.Stderr
			_ = cmd.Run() // Ignore oxfmt errors
		}
	}

	return nil
}

// findOxfmt returns the oxfmt binary to run: the project's node_modules copy
// when present, otherwise oxfmt resolved from PATH. This formats generated
// TypeScript the same whether or not the caller put node_modules/.bin on PATH.
func findOxfmt(projectDir string) string {
	local := filepath.Join(projectDir, "node_modules", ".bin", "oxfmt")
	if _, err := os.Stat(local); err == nil {
		return local
	}
	return "oxfmt"
}

// Clean removes the generated files of every run in the selected manifest.
func (g *Generator) Clean() error {
	// Load the complete inventory before removing the manifest.
	cacheFile, err := g.Config.GetCacheFilePath()
	if err != nil {
		return err
	}
	manifest, err := LoadCache(cacheFile)
	if err != nil {
		return errors.Wrap(err, "failed to load cache")
	}
	_ = os.Remove(cacheFile)

	// Remove outputs from each recorded generation selection.
	for _, run := range manifest.Runs {
		if run == nil {
			continue
		}
		for _, pkg := range run.Packages {
			if pkg == nil {
				continue
			}
			for _, f := range pkg.GeneratedFiles {
				_ = os.Remove(filepath.Join(g.ProjectDir, f))
			}
		}
	}

	return nil
}
