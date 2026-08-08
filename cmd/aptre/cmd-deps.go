package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/common/protogen"
)

// Tool definitions.
type toolSpec struct {
	Name       string
	ImportPath string
	ModulePath string
}

var defaultTools = []toolSpec{
	{Name: "protoc-gen-go-lite", ImportPath: "github.com/aperturerobotics/protobuf-go-lite/cmd/protoc-gen-go-lite", ModulePath: "github.com/aperturerobotics/protobuf-go-lite"},
	{Name: "protoc-gen-go-starpc", ImportPath: "github.com/aperturerobotics/starpc/cmd/protoc-gen-go-starpc", ModulePath: "github.com/aperturerobotics/starpc"},
	{Name: "protoc-gen-starpc-cpp", ImportPath: "github.com/aperturerobotics/starpc/cmd/protoc-gen-starpc-cpp", ModulePath: "github.com/aperturerobotics/starpc"},
	{Name: "protoc-gen-starpc-rust", ImportPath: "github.com/aperturerobotics/starpc/cmd/protoc-gen-starpc-rust", ModulePath: "github.com/aperturerobotics/starpc"},
	{Name: "gofumpt", ImportPath: "mvdan.cc/gofumpt"}, {Name: "goimports", ImportPath: "golang.org/x/tools/cmd/goimports"},
	{Name: "golangci-lint", ImportPath: "github.com/golangci/golangci-lint/v2/cmd/golangci-lint"}, {Name: "go-mod-outdated", ImportPath: "github.com/psampaz/go-mod-outdated"},
	{Name: "goreleaser", ImportPath: "github.com/goreleaser/goreleaser/v2"}, {Name: "wasmbrowsertest", ImportPath: "github.com/agnivade/wasmbrowsertest"},
}

type toolBuildMode uint8

const (
	toolBuildIsolated toolBuildMode = iota
	toolBuildVersioned
)

type toolBuildPlan struct {
	mode    toolBuildMode
	spec    toolSpec
	version string
}

func toolSpecFor(name string) (toolSpec, bool) {
	for _, spec := range defaultTools {
		if spec.Name == name {
			return spec, true
		}
	}
	return toolSpec{}, false
}

func selectedToolPlan(projectDir, name string) toolBuildPlan {
	spec, ok := toolSpecFor(name)
	if !ok || spec.ModulePath == "" {
		return toolBuildPlan{mode: toolBuildIsolated, spec: spec}
	}
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Path}}\t{{.Version}}\t{{.Main}}", spec.ModulePath)
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

type generateDependencyPlan struct {
	nativeTools       []string
	ensureNodeModules bool
}

func planGenerateDependencies(cfg *protogen.Config) (generateDependencyPlan, error) {
	langs, err := cfg.GetLanguages()
	if err != nil {
		return generateDependencyPlan{}, err
	}
	rpcs, err := cfg.GetRPCLibraries()
	if err != nil {
		return generateDependencyPlan{}, err
	}

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
	hasPackageJSON, err := cfg.HasPackageJSON()
	if err != nil {
		return generateDependencyPlan{}, err
	}
	return generateDependencyPlan{
		nativeTools:       tools,
		ensureNodeModules: hasPackageJSON && langs.Has(protogen.LanguageTypeScript),
	}, nil
}

func ensureGenerateDeps(cfg *protogen.Config, verbose bool) error {
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

func runDeps(c *cli.Context) error {
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

func ensureAllDeps(projectDir, toolsDir string, verbose, force bool) error {
	absProjectDir, err := filepath.Abs(projectDir)
	if err != nil {
		return err
	}

	// Ensure tools directory exists
	toolsPath := filepath.Join(absProjectDir, toolsDir)
	if err := ensureToolsDir(absProjectDir, toolsPath, verbose); err != nil {
		return err
	}

	// Build required tools
	requiredTools := []string{"protoc-gen-go-lite", "protoc-gen-go-starpc", "protoc-gen-starpc-cpp", "protoc-gen-starpc-rust", "gofumpt"}
	for _, toolName := range requiredTools {
		if err := ensureTool(absProjectDir, toolsPath, toolName, force, verbose); err != nil {
			return fmt.Errorf("failed to ensure %s: %w", toolName, err)
		}
	}

	// Ensure node_modules if package.json exists
	if _, err := os.Stat(filepath.Join(absProjectDir, "package.json")); err == nil {
		if err := ensureNodeModules(absProjectDir, verbose); err != nil {
			return fmt.Errorf("failed to ensure node_modules: %w", err)
		}
	}

	return nil
}

func toolsStampPath(toolsPath string) string {
	return filepath.Join(toolsPath, ".common-tools-stamp")
}

func reconcileToolsStamp(stampPath, identity string, extract func() error) (bool, error) {
	if data, err := os.ReadFile(stampPath); err == nil && strings.TrimSpace(string(data)) == identity {
		return false, nil
	}
	if err := extract(); err != nil {
		return false, err
	}
	if err := os.WriteFile(stampPath, []byte(identity+"\n"), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

func ensureToolsDir(projectDir, toolsPath string, verbose bool) error {
	if err := os.MkdirAll(toolsPath, 0o755); err != nil {
		return err
	}
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
	})
	return err
}

func resolveCommonPackage(projectDir string) string {
	const commonModule = "github.com/aperturerobotics/common"
	const moduleTemplate = "{{with .Replace}}{{if .Version}}{{.Path}}@{{.Version}}{{else}}{{.Path}}{{end}}{{else}}{{if .Version}}{{.Path}}@{{.Version}}{{else}}{{.Path}}{{end}}{{end}}"

	cmd := exec.Command("go", "list", "-m", "-f", moduleTemplate, commonModule)
	cmd.Dir = projectDir
	output, err := cmd.Output()
	if err == nil {
		module := strings.TrimSpace(string(output))
		if module != "" && module != "<nil>" {
			return module
		}
	}

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

func ensureTool(projectDir, toolsPath, toolName string, force, verbose bool) error {
	binPath := filepath.Join(toolsPath, "bin", toolName)

	// Check if already exists
	if !force {
		if _, err := os.Stat(binPath); err == nil {
			return nil
		}
	}

	spec, ok := toolSpecFor(toolName)
	if !ok {
		return fmt.Errorf("unknown tool: %s", toolName)
	}

	if verbose {
		fmt.Printf("Building %s...\n", toolName)
	}

	plan := selectedToolPlan(projectDir, toolName)
	var cmd *exec.Cmd
	if plan.mode == toolBuildVersioned {
		cmd = exec.Command("go", "install", spec.ImportPath+"@"+plan.version)
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

func ensureNodeModules(projectDir string, verbose bool) error {
	nodeModulesPath := filepath.Join(projectDir, "node_modules")
	if _, err := os.Stat(nodeModulesPath); err == nil {
		return nil // Already exists
	}

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

	// Ensure tools directory exists first
	if err := ensureToolsDir(absProjectDir, toolsPath, verbose); err != nil {
		return "", fmt.Errorf("failed to ensure tools directory: %w", err)
	}

	if err := ensureTool(absProjectDir, toolsPath, toolName, false, verbose); err != nil {
		return "", err
	}

	if toolName == "golangci-lint" {
		if err := maybeBuildCustomGolangCILint(absProjectDir, toolsPath, verbose); err != nil {
			return "", err
		}
	}

	return filepath.Join(toolsPath, "bin", toolName), nil
}

func maybeBuildCustomGolangCILint(projectDir, toolsPath string, verbose bool) error {
	customConfPath := filepath.Join(projectDir, ".custom-gcl.yml")
	customConfDat, err := os.ReadFile(customConfPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	version := parseCustomGolangCILintVersion(string(customConfDat))
	if version == "" {
		return fmt.Errorf("missing version in %s", customConfPath)
	}
	baseLintPath := filepath.Join(toolsPath, "bin", "golangci-lint")
	customStampPath := filepath.Join(toolsPath, "bin", ".golangci-lint-custom-stamp")
	customStamp := strings.Join([]string{version, customConfPath}, "\n")
	if stampDat, err := os.ReadFile(customStampPath); err == nil && string(stampDat) == customStamp {
		return nil
	}
	builderPath := filepath.Join(toolsPath, "bin", "golangci-lint-builder")
	if err := os.Rename(baseLintPath, builderPath); err != nil {
		return err
	}
	defer func() {
		if _, err := os.Stat(baseLintPath); err != nil {
			_ = os.Rename(builderPath, baseLintPath)
		}
	}()
	args := []string{
		"custom",
		"--name", "golangci-lint",
		"--destination", filepath.Join(toolsPath, "bin"),
		"--version", version,
	}
	cmd := exec.Command(builderPath, args...)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if verbose {
		fmt.Printf("Building custom golangci-lint from %s...\n", customConfPath)
	}
	if err := cmd.Run(); err != nil {
		_ = os.Rename(builderPath, baseLintPath)
		return err
	}
	if err := os.Remove(builderPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(customStampPath, []byte(customStamp), 0o644) //nolint:gosec
}

func parseCustomGolangCILintVersion(conf string) string {
	for line := range strings.SplitSeq(conf, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "version:") {
			continue
		}
		version := strings.TrimSpace(strings.TrimPrefix(line, "version:"))
		return strings.Trim(version, `"'`)
	}
	return ""
}
