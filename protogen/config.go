package protogen

import (
	"os"
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
	projectDir, err := c.GetProjectDir()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(filepath.Join(projectDir, "go.mod"))
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
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

// GetGoModule returns the Go module path from go.mod.
func (c *Config) GetGoModule() (string, error) {
	projectDir, err := c.GetProjectDir()
	if err != nil {
		return "", err
	}
	return GetGoModule(projectDir)
}
