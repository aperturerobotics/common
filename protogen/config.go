package protogen

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
)

// DefaultCacheFile is the default cache file name.
const DefaultCacheFile = ".protoc-manifest.json"

// DefaultGoLiteFeatures is the default set of go-lite features to enable.
const DefaultGoLiteFeatures = "marshal+unmarshal+size+equal+json+clone+text"

// Config contains the configuration for proto generation.
type Config struct {
	// ProjectDir is the project directory.
	// If empty, uses the current working directory.
	ProjectDir string
	// Targets is the list of proto file glob patterns to process.
	// Default: ["./*.proto"]
	Targets []string
	// Exclude is a list of proto file glob patterns to exclude.
	// Files matching any of these patterns will be skipped.
	Exclude []string
	// Force regenerates all files regardless of cache.
	Force bool
	// CacheFile is the path to the cache file.
	// Default: ".protoc-manifest.json"
	CacheFile string
	// Verbose enables verbose output.
	Verbose bool
	// GoLiteFeatures is the go-lite features to enable.
	// Default: "marshal+unmarshal+size+equal+json+clone+text"
	GoLiteFeatures string
	// ToolsDir is the tools directory containing plugin binaries.
	// Default: ".tools"
	ToolsDir string
	// ExtraArgs contains any additional protoc arguments.
	ExtraArgs []string
}

// NewConfig returns a new Config with default values.
func NewConfig() *Config {
	return &Config{
		Targets:        []string{"./*.proto"},
		CacheFile:      DefaultCacheFile,
		GoLiteFeatures: DefaultGoLiteFeatures,
		ToolsDir:       ".tools",
	}
}

// GetProjectDir returns the project directory, defaulting to cwd.
func (c *Config) GetProjectDir() (string, error) {
	if c.ProjectDir != "" {
		return filepath.Abs(c.ProjectDir)
	}
	return os.Getwd()
}

// GetModuleDir returns the nearest ancestor directory containing go.mod.
func (c *Config) GetModuleDir() (string, error) {
	projectDir, err := c.GetProjectDir()
	if err != nil {
		return "", err
	}
	return FindModuleDir(projectDir)
}

// GetCacheFilePath returns the absolute path to the cache file.
func (c *Config) GetCacheFilePath() (string, error) {
	projectDir, err := c.GetProjectDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(projectDir, c.CacheFile), nil
}

// GetToolsDir returns the absolute path to the tools directory.
func (c *Config) GetToolsDir() (string, error) {
	projectDir, err := c.GetProjectDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(projectDir, c.ToolsDir), nil
}

// HasGoMod checks if go.mod exists in the project directory.
func (c *Config) HasGoMod() (bool, error) {
	_, err := c.GetModuleDir()
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// HasPackageJSON checks if package.json exists in the project directory.
func (c *Config) HasPackageJSON() (bool, error) {
	projectDir, err := c.GetProjectDir()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(filepath.Join(projectDir, "package.json"))
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

// GetGoModule returns the effective Go import path for the project directory.
func (c *Config) GetGoModule() (string, error) {
	projectDir, err := c.GetProjectDir()
	if err != nil {
		return "", err
	}

	moduleDir, err := c.GetModuleDir()
	if err != nil {
		return "", err
	}

	modulePath, err := GetGoModule(moduleDir)
	if err != nil {
		return "", err
	}

	projectRel, err := filepath.Rel(moduleDir, projectDir)
	if err != nil {
		return "", err
	}
	if projectRel == "." {
		return modulePath, nil
	}
	return path.Join(modulePath, filepath.ToSlash(projectRel)), nil
}

// FindModuleDir finds the nearest ancestor directory containing go.mod.
func FindModuleDir(projectDir string) (string, error) {
	dir := projectDir
	for {
		_, err := os.Stat(filepath.Join(dir, "go.mod"))
		if err == nil {
			return dir, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found in %s or ancestors: %w", projectDir, os.ErrNotExist)
		}
		dir = parent
	}
}
