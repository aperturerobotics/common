package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/aperturerobotics/cli"
)

// Tool definitions
var defaultTools = []struct {
	Name       string
	ImportPath string
}{
	{"protoc-gen-go-lite", "github.com/aperturerobotics/protobuf-go-lite/cmd/protoc-gen-go-lite"},
	{"protoc-gen-go-starpc", "github.com/aperturerobotics/starpc/cmd/protoc-gen-go-starpc"},
	{"protoc-gen-starpc-cpp", "github.com/aperturerobotics/starpc/cmd/protoc-gen-starpc-cpp"},
	{"protoc-gen-starpc-rust", "github.com/aperturerobotics/starpc/cmd/protoc-gen-starpc-rust"},
	{"gofumpt", "mvdan.cc/gofumpt"},
	{"goimports", "golang.org/x/tools/cmd/goimports"},
	{"golangci-lint", "github.com/golangci/golangci-lint/v2/cmd/golangci-lint"},
	{"go-mod-outdated", "github.com/psampaz/go-mod-outdated"},
	{"goreleaser", "github.com/goreleaser/goreleaser/v2"},
	{"wasmbrowsertest", "github.com/agnivade/wasmbrowsertest"},
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
		if err := ensureTool(toolsPath, toolName, force, verbose); err != nil {
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

func ensureToolsDir(projectDir, toolsPath string, verbose bool) error {
	if _, err := os.Stat(toolsPath); err == nil {
		return nil // Already exists
	}

	if verbose {
		fmt.Println("Setting up tools directory...")
	}

	relToolsPath, err := filepath.Rel(projectDir, toolsPath)
	if err != nil {
		relToolsPath = filepath.Base(toolsPath)
	}

	commonPackage := resolveCommonPackage(projectDir)
	cmd := exec.Command("go", "run", "-v", commonPackage, relToolsPath)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
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

func ensureTool(toolsPath, toolName string, force, verbose bool) error {
	binPath := filepath.Join(toolsPath, "bin", toolName)

	// Check if already exists
	if !force {
		if _, err := os.Stat(binPath); err == nil {
			return nil
		}
	}

	// Find the import path for this tool
	var importPath string
	for _, t := range defaultTools {
		if t.Name == toolName {
			importPath = t.ImportPath
			break
		}
	}
	if importPath == "" {
		return fmt.Errorf("unknown tool: %s", toolName)
	}

	if verbose {
		fmt.Printf("Building %s...\n", toolName)
	}

	// Build the tool
	// #nosec G204 -- toolName and importPath come from hardcoded defaultTools list
	cmd := exec.Command("go", "build", "-mod=readonly", "-v", "-o", filepath.Join("bin", toolName), importPath)
	cmd.Dir = toolsPath
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

	if err := ensureTool(toolsPath, toolName, false, verbose); err != nil {
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
