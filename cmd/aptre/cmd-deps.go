package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/common/golangcilint"
	"github.com/aperturerobotics/common/protogen"
)

// toolSpec identifies an executable and the module that selects its version.
type toolSpec struct {
	// Name is the executable name under the tools bin directory.
	Name string
	// ImportPath is the Go main package used to build the executable.
	ImportPath string
	// ModulePath selects the project dependency when the tool follows its version.
	ModulePath string
}

// defaultTools lists the supported build tools and their import identities.
var defaultTools = []toolSpec{
	{Name: "protoc-gen-go-lite", ImportPath: "github.com/aperturerobotics/protobuf-go-lite/cmd/protoc-gen-go-lite", ModulePath: "github.com/aperturerobotics/protobuf-go-lite"},
	{Name: "protoc-gen-go-starpc", ImportPath: "github.com/aperturerobotics/starpc/cmd/protoc-gen-go-starpc", ModulePath: "github.com/aperturerobotics/starpc"},
	{Name: "protoc-gen-starpc-cpp", ImportPath: "github.com/aperturerobotics/starpc/cmd/protoc-gen-starpc-cpp", ModulePath: "github.com/aperturerobotics/starpc"},
	{Name: "protoc-gen-starpc-rust", ImportPath: "github.com/aperturerobotics/starpc/cmd/protoc-gen-starpc-rust", ModulePath: "github.com/aperturerobotics/starpc"},
	{Name: "gofumpt", ImportPath: "mvdan.cc/gofumpt"},
	{Name: "goimports", ImportPath: "golang.org/x/tools/cmd/goimports"},
	{Name: "golangci-lint", ImportPath: "github.com/golangci/golangci-lint/v2/cmd/golangci-lint"},
	{Name: "go-mod-outdated", ImportPath: "github.com/psampaz/go-mod-outdated"},
	{Name: "goreleaser", ImportPath: "github.com/goreleaser/goreleaser/v2"},
	{Name: "wasmbrowsertest", ImportPath: "github.com/agnivade/wasmbrowsertest"},
}

// toolBuildMode selects the isolated tool module or a project-pinned version.
type toolBuildMode uint8

const (
	// toolBuildIsolated builds from the extracted tools module.
	toolBuildIsolated toolBuildMode = iota
	// toolBuildVersioned installs the version selected by the project module.
	toolBuildVersioned
)

// toolBuildPlan captures the selected source for one executable.
type toolBuildPlan struct {
	// mode determines whether the tools module or project selects dependencies.
	mode toolBuildMode
	// spec identifies the executable and main package.
	spec toolSpec
	// version is the project-selected module version for a versioned build.
	version string
}

// toolSpecFor resolves an executable against the supported tool table.
func toolSpecFor(name string) (toolSpec, bool) {
	for _, spec := range defaultTools {
		if spec.Name == name {
			return spec, true
		}
	}
	return toolSpec{}, false
}

// selectedToolPlan follows a project dependency when the tool supports it.
func selectedToolPlan(projectDir, name string) toolBuildPlan {
	// Tools without a shared project module use the isolated tool dependencies.
	spec, ok := toolSpecFor(name)
	if !ok || spec.ModulePath == "" {
		return toolBuildPlan{mode: toolBuildIsolated, spec: spec}
	}

	// Resolve the project's selected version, preserving fallback on lookup errors.
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Path}}\t{{.Version}}\t{{.Main}}", spec.ModulePath) //nolint:gosec // spec comes from the fixed tool table.
	cmd.Dir = projectDir
	out, err := cmd.Output()
	if err != nil {
		return toolBuildPlan{mode: toolBuildIsolated, spec: spec}
	}
	parts := strings.Split(strings.TrimSpace(string(out)), "\t")
	if len(parts) < 3 {
		return toolBuildPlan{mode: toolBuildIsolated, spec: spec}
	}
	if parts[2] == "true" {
		return toolBuildPlan{mode: toolBuildIsolated, spec: spec}
	}
	if parts[1] != "" {
		return toolBuildPlan{mode: toolBuildVersioned, spec: spec, version: parts[1]}
	}
	return toolBuildPlan{mode: toolBuildIsolated, spec: spec}
}

// generateDependencyPlan limits setup to the configured generation languages.
type generateDependencyPlan struct {
	// nativeTools lists the required executables in build order.
	nativeTools []string
	// ensureNodeModules includes JavaScript dependencies when generation needs them.
	ensureNodeModules bool
}

// planGenerateDependencies selects tools for the requested languages and RPCs.
func planGenerateDependencies(cfg *protogen.Config) (generateDependencyPlan, error) {
	// Read the language and RPC contracts before choosing their generators.
	langs, err := cfg.GetLanguages()
	if err != nil {
		return generateDependencyPlan{}, err
	}
	rpcs, err := cfg.GetRPCLibraries()
	if err != nil {
		return generateDependencyPlan{}, err
	}

	// Select only the native plugins used by the configured output languages.
	var tools []string
	if langs.Has(protogen.LanguageGo) {
		tools = append(tools, "protoc-gen-go-lite")
		if rpcs.Has(protogen.RPCLibraryStarpc) {
			tools = append(tools, "protoc-gen-go-starpc")
		}
		tools = append(tools, "gofumpt")
	}
	if langs.Has(protogen.LanguageCpp) && rpcs.Has(protogen.RPCLibraryStarpc) {
		tools = append(tools, "protoc-gen-starpc-cpp")
	}
	if langs.Has(protogen.LanguageRust) && rpcs.Has(protogen.RPCLibraryStarpc) {
		tools = append(tools, "protoc-gen-starpc-rust")
	}
	// TypeScript generation also needs the project's package dependencies.
	hasPackageJSON, err := cfg.HasPackageJSON()
	if err != nil {
		return generateDependencyPlan{}, err
	}
	return generateDependencyPlan{
		nativeTools:       tools,
		ensureNodeModules: hasPackageJSON && langs.Has(protogen.LanguageTypeScript),
	}, nil
}

// ensureGenerateDeps prepares only dependencies required by this generation.
func ensureGenerateDeps(cfg *protogen.Config, verbose bool) error {
	// Resolve the dependency plan and build each required native executable.
	plan, err := planGenerateDependencies(cfg)
	if err != nil {
		return err
	}
	if len(plan.nativeTools) != 0 {
		projectDir, err := cfg.GetProjectDir()
		if err != nil {
			return err
		}
		toolsDir, err := cfg.GetToolsDir()
		if err != nil {
			return err
		}
		toolsPath := toolsDir
		if err := ensureToolsDir(projectDir, toolsPath, verbose); err != nil {
			return err
		}
		for _, tool := range plan.nativeTools {
			if err := ensureTool(projectDir, toolsPath, tool, false, verbose); err != nil {
				return err
			}
		}
	}

	// Install package dependencies when the selected languages require them.
	if plan.ensureNodeModules {
		projectDir, err := cfg.GetProjectDir()
		if err != nil {
			return err
		}
		if err := ensureNodeModules(projectDir, verbose); err != nil {
			return err
		}
	}
	return nil
}

// depsCmd exposes explicit tool and package dependency preparation.
var depsCmd = &cli.Command{
	Name:    "deps",
	Aliases: []string{"protodeps"},
	Usage:   "Ensure all dependencies are installed",
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
		&cli.BoolFlag{
			Name:  "force",
			Usage: "Force rebuild of all tools",
		},
	},
	Action: runDeps,
}

// runDeps resolves command flags and prepares the selected project.
func runDeps(c *cli.Context) error {
	// Read the dependency command's project and build options.
	projectDir := c.String("project-dir")
	toolsDir := c.String("tools-dir")
	verbose := c.Bool("verbose")
	force := c.Bool("force")

	if projectDir == "" {
		var err error
		projectDir, err = os.Getwd()
		if err != nil {
			return err
		}
	}

	return ensureAllDeps(projectDir, toolsDir, verbose, force)
}

// ensureDeps prepares all dependencies without forcing existing tool rebuilds.
func ensureDeps(projectDir, toolsDir string, verbose bool) error {
	if projectDir == "" {
		var err error
		projectDir, err = os.Getwd()
		if err != nil {
			return err
		}
	}
	return ensureAllDeps(projectDir, toolsDir, verbose, false)
}

// ensureAllDeps prepares the native tools and any project package dependencies.
func ensureAllDeps(projectDir, toolsDir string, verbose, force bool) error {
	// Resolve the project root and synchronize its embedded tool metadata.
	absProjectDir, err := filepath.Abs(projectDir)
	if err != nil {
		return err
	}

	toolsPath := filepath.Join(absProjectDir, toolsDir)
	if err := ensureToolsDir(absProjectDir, toolsPath, verbose); err != nil {
		return err
	}

	// Build the generators and formatter required by explicit dependency setup.
	requiredTools := []string{"protoc-gen-go-lite", "protoc-gen-go-starpc", "protoc-gen-starpc-cpp", "protoc-gen-starpc-rust", "gofumpt"}
	for _, toolName := range requiredTools {
		if err := ensureTool(absProjectDir, toolsPath, toolName, force, verbose); err != nil {
			return fmt.Errorf("failed to ensure %s: %w", toolName, err)
		}
	}

	// Install package dependencies when this project declares them.
	if _, err := os.Stat(filepath.Join(absProjectDir, "package.json")); err == nil {
		if err := ensureNodeModules(absProjectDir, verbose); err != nil {
			return fmt.Errorf("failed to ensure node_modules: %w", err)
		}
	}

	return nil
}

// toolsStampPath locates the identity of the extracted common tool metadata.
func toolsStampPath(toolsPath string) string {
	return filepath.Join(toolsPath, ".common-tools-stamp")
}

// reconcileToolsStamp extracts changed metadata before invalidating its binaries.
func reconcileToolsStamp(stampPath, identity string, extract func() error, invalidate func() error) (bool, error) {
	// Reuse metadata only while its resolved common module identity matches.
	if data, err := os.ReadFile(stampPath); err == nil && strings.TrimSpace(string(data)) == identity {
		return false, nil
	}

	// Publish the new stamp only after extraction and invalidation succeed.
	if err := extract(); err != nil {
		return false, err
	}
	if invalidate != nil {
		if err := invalidate(); err != nil {
			return false, err
		}
	}
	if err := os.WriteFile(stampPath, []byte(identity+"\n"), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// customGolangCIStampPath locates the fingerprint of the custom linter build.
func customGolangCIStampPath(toolsPath string) string {
	return filepath.Join(toolsPath, "bin", ".golangci-lint-custom-stamp")
}

// invalidateToolBinaries removes stale executables and their custom-build stamp.
func invalidateToolBinaries(toolsPath string) error {
	// Collect the fixed executable set affected by changed tool metadata.
	paths := make([]string, 0, len(defaultTools)+1)
	for _, spec := range defaultTools {
		paths = append(paths, filepath.Join(toolsPath, "bin", spec.Name))
	}

	// The custom golangci-lint build replaces bin/golangci-lint and records its
	// version and config in this stamp. Removing the binary while keeping the
	// stamp makes the next ensureTool rebuild the stock binary, which cannot
	// resolve the configured plugin linters.
	paths = append(paths, customGolangCIStampPath(toolsPath))
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// ensureToolsDir reconciles extracted tool metadata with the selected common module.
func ensureToolsDir(projectDir, toolsPath string, verbose bool) error {
	// Create the tool directory before reading or publishing its metadata stamp.
	if err := os.MkdirAll(toolsPath, 0o755); err != nil {
		return err
	}

	// Extract the selected common module and invalidate binaries from older inputs.
	identity := resolveCommonPackage(projectDir)
	_, err := reconcileToolsStamp(toolsStampPath(toolsPath), identity, func() error {
		if verbose {
			fmt.Println("Synchronizing embedded tool metadata...")
		}
		relToolsPath, err := filepath.Rel(projectDir, toolsPath)
		if err != nil {
			relToolsPath = filepath.Base(toolsPath)
		}
		cmd := exec.Command("go", "run", "-mod=mod", "-v", identity, relToolsPath)
		cmd.Dir = projectDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}, func() error {
		return invalidateToolBinaries(toolsPath)
	})
	return err
}

// resolveCommonPackage follows project replacements before falling back to build metadata.
func resolveCommonPackage(projectDir string) string {
	const commonModule = "github.com/aperturerobotics/common"
	const moduleTemplate = "{{with .Replace}}{{if .Version}}{{.Path}}@{{.Version}}{{else}}{{.Path}}{{end}}{{else}}{{if .Version}}{{.Path}}@{{.Version}}{{else}}{{.Path}}{{end}}{{end}}"

	// Prefer the module selected by the target project, including replacements.
	cmd := exec.Command("go", "list", "-m", "-f", moduleTemplate, commonModule)
	cmd.Dir = projectDir
	output, err := cmd.Output()
	if err == nil {
		module := strings.TrimSpace(string(output))
		if module != "" && module != "<nil>" {
			return module
		}
	}

	// Installed aptre binaries retain their common version in build metadata.
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			return commonModule + "@" + info.Main.Version
		}
		for _, dep := range info.Deps {
			if dep.Path == commonModule {
				version := dep.Version
				if dep.Replace != nil && dep.Replace.Version != "" {
					version = dep.Replace.Version
				}
				if version != "" && version != "(devel)" {
					return commonModule + "@" + version
				}
			}
		}
	}

	return commonModule
}

// ensureTool builds a missing executable using its resolved dependency plan.
func ensureTool(projectDir, toolsPath, toolName string, force, verbose bool) error {
	// Preserve existing executables unless explicitly rebuilding them.
	binPath := filepath.Join(toolsPath, "bin", toolName)
	if !force {
		if _, err := os.Stat(binPath); err == nil {
			return nil
		}
	}

	// Resolve a supported tool before selecting its build source.
	spec, ok := toolSpecFor(toolName)
	if !ok {
		return fmt.Errorf("unknown tool: %s", toolName)
	}

	if verbose {
		fmt.Printf("Building %s...\n", toolName)
	}

	// Select the project-pinned installation or the isolated tools module build.
	plan := selectedToolPlan(projectDir, toolName)
	var cmd *exec.Cmd
	if plan.mode == toolBuildVersioned {
		cmd = exec.Command("go", "install", spec.ImportPath+"@"+plan.version) //nolint:gosec // spec and version come from the fixed tool plan.
		cmd.Dir = projectDir
		cmd.Env = append(os.Environ(), "GOBIN="+filepath.Join(toolsPath, "bin"))
	} else {
		cmd = exec.Command("go", "build", "-mod=readonly", "-v", "-o", binPath, spec.ImportPath)
		cmd.Dir = toolsPath
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ensureNodeModules installs dependencies only when node_modules is absent.
func ensureNodeModules(projectDir string, verbose bool) error {
	// Reuse a completed installation in the project directory.
	nodeModulesPath := filepath.Join(projectDir, "node_modules")
	if _, err := os.Stat(nodeModulesPath); err == nil {
		return nil // Already exists
	}

	// Forward installation output so package-manager failures remain actionable.
	if verbose {
		fmt.Println("Installing node_modules...")
	}

	cmd := exec.Command("bun", "install")
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// EnsureToolBuilt ensures a specific tool is built and returns its path.
func EnsureToolBuilt(projectDir, toolsDir, toolName string, verbose bool) (string, error) {
	// Resolve the project and its selected tools directory.
	if projectDir == "" {
		var err error
		projectDir, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}

	absProjectDir, err := filepath.Abs(projectDir)
	if err != nil {
		return "", err
	}

	toolsPath := filepath.Join(absProjectDir, toolsDir)

	// Synchronize metadata before reusing or building an executable.
	if err := ensureToolsDir(absProjectDir, toolsPath, verbose); err != nil {
		return "", fmt.Errorf("failed to ensure tools directory: %w", err)
	}

	if err := ensureTool(absProjectDir, toolsPath, toolName, false, verbose); err != nil {
		return "", err
	}

	// Compose custom linter modules after the builder executable is available.
	if toolName == "golangci-lint" {
		if err := maybeBuildCustomGolangCILint(absProjectDir, toolsPath, verbose); err != nil {
			return "", err
		}
	}

	return filepath.Join(toolsPath, "bin", toolName), nil
}

// maybeBuildCustomGolangCILint rebuilds the linter when its plugin inputs change.
func maybeBuildCustomGolangCILint(projectDir, toolsPath string, verbose bool) error {
	// Read optional plugin configuration before attempting a custom build.
	customConfPath := filepath.Join(projectDir, ".custom-gcl.yml")
	customConfDat, err := os.ReadFile(customConfPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	// Reuse a custom binary only while its configuration and plugin files match.
	baseLintPath := filepath.Join(toolsPath, "bin", "golangci-lint")
	customStampPath := customGolangCIStampPath(toolsPath)
	customStamp, err := golangcilint.Fingerprint(customConfPath, customConfDat)
	if err != nil {
		return err
	}
	if stampDat, err := os.ReadFile(customStampPath); err == nil && string(stampDat) == customStamp {
		return nil
	}

	// Build beside the installed tool so failure leaves that executable intact.
	buildDir, err := os.MkdirTemp(toolsPath, ".golangci-lint-build-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(buildDir)
	buildConf, err := golangcilint.BuildConfig(customConfPath, customConfDat, buildDir)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(buildDir, ".custom-gcl.yml"), buildConf, 0o600); err != nil {
		return err
	}

	// The installed custom binary can be older than the tools module and cannot
	// reliably rebuild itself. Compile the builder selected by the tools module.
	builderPath := filepath.Join(buildDir, "builder")
	cmd := exec.Command("go", "build", "-mod=readonly", "-o", builderPath, "github.com/golangci/golangci-lint/v2/cmd/golangci-lint")
	cmd.Dir = toolsPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	// Configuration fields keep plugin paths and output independent of the CWD.
	cmd = exec.Command(builderPath, "custom")
	if verbose {
		cmd.Args = append(cmd.Args, "--verbose")
	}
	cmd.Dir = buildDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if verbose {
		fmt.Printf("Building custom golangci-lint from %s...\n", customConfPath)
	}
	if err := cmd.Run(); err != nil {
		return err
	}

	// Publish the executable before marking its inputs as current.
	if err := os.Rename(filepath.Join(buildDir, "golangci-lint"), baseLintPath); err != nil {
		return err
	}
	return os.WriteFile(customStampPath, []byte(customStamp), 0o644) //nolint:gosec
}
