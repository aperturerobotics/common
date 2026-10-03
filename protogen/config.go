package protogen

import (
	"os"
	"path"
	"path/filepath"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
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
	// Languages is the opt-in protobuf output language filter.
	// Empty preserves the Go, TypeScript, C++, and Rust defaults.
	Languages []string
	// RPCLibraries is the opt-in RPC stub generator filter.
	// Empty enables the default RPC library set.
	RPCLibraries []string
	// TsImportBoundaries are module-relative path prefixes where generated
	// TypeScript protobuf imports should switch to @go/... when crossing
	// between boundaries.
	TsImportBoundaries []string
}

// RustConfig configures whole-graph Rust generation.
//
// Every path is slash-separated and relative to the project directory, which
// is also the Rust crate root.
type RustConfig struct {
	// ProstOptions are protoc-gen-prost options forwarded verbatim, such as
	// "btree_map=." or "extern_path=.pkg=::crate::pkg".
	ProstOptions []string
	// Exclude lists proto path patterns to skip, in addition to Config.Exclude.
	Exclude []string
	// DescriptorSet is where the encoded FileDescriptorSet of the whole graph,
	// with imports and source info, is written. Empty skips the descriptor set.
	DescriptorSet string
	// ModuleFile is where the module declarations are written. Each generated
	// file is included under the Rust module of its protobuf package. Empty
	// skips the module file.
	ModuleFile string
	// Inventory lists every generated file so that outputs of deleted schemas
	// can be removed. Default: DefaultRustInventory.
	Inventory string
}

// DefaultRustInventory is the default generated-file inventory for Rust output.
const DefaultRustInventory = ".protoc-rust-files.txt"

// packageJSONAptreConfig is the typed generation configuration read from package.json.
type packageJSONAptreConfig struct {
	// Module is the canonical protobuf import prefix.
	Module string
	// Languages selects the generated languages.
	Languages []string
	// RPCLibraries selects the service generators.
	RPCLibraries []string
	// TsImportBoundaries selects TypeScript package crossings.
	TsImportBoundaries []string
	// Rust enables whole-graph Rust generation when present.
	Rust *RustConfig
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
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// HasPackageJSON checks if package.json exists in the project directory.
func (c *Config) HasPackageJSON() (bool, error) {
	// Resolve the project before checking its package manifest.
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
	// Resolve the project independently of its containing Go module.
	projectDir, err := c.GetProjectDir()
	if err != nil {
		return "", err
	}

	// Locate the module that supplies the import prefix.
	moduleDir, err := c.GetModuleDir()
	if err != nil {
		return "", err
	}

	// Read the declared module identity rather than inferring it from the directory.
	modulePath, err := GetGoModule(moduleDir)
	if err != nil {
		return "", err
	}

	// Subprojects extend the module identity with their relative directory.
	projectRel, err := filepath.Rel(moduleDir, projectDir)
	if err != nil {
		return "", err
	}
	if projectRel == "." {
		return modulePath, nil
	}
	return path.Join(modulePath, filepath.ToSlash(projectRel)), nil
}

// readAptreConfig reads the aptre section of package.json.
// It returns an empty section when package.json or the section is absent.
func (c *Config) readAptreConfig() (*packageJSONAptreConfig, error) {
	// Select the package manifest of this project, not an ancestor module.
	projectDir, err := c.GetProjectDir()
	if err != nil {
		return nil, err
	}

	// Absence selects defaults; unreadable manifests remain errors.
	data, err := os.ReadFile(filepath.Join(projectDir, "package.json"))
	if os.IsNotExist(err) {
		return &packageJSONAptreConfig{}, nil
	}
	if err != nil {
		return nil, err
	}

	// Parse only the typed aptre section while accepting other package metadata.
	value, err := fastjson.ParseBytes(data)
	if err != nil {
		return nil, err
	}
	if value.Type() != fastjson.TypeNull {
		if _, err := value.Object(); err != nil {
			return nil, err
		}
	}
	config := &packageJSONAptreConfig{}
	if err := config.FromJSON(value.Get("aptre")); err != nil {
		return nil, err
	}
	return config, nil
}

// GetTsImportBoundaries returns configured TypeScript import boundaries.
// Explicit config takes precedence; otherwise reads package.json aptre config.
func (c *Config) GetTsImportBoundaries() ([]string, error) {
	if len(c.TsImportBoundaries) != 0 {
		return c.TsImportBoundaries, nil
	}

	aptre, err := c.readAptreConfig()
	if err != nil {
		return nil, err
	}
	return aptre.TsImportBoundaries, nil
}

// GetLanguages returns configured output languages.
// Explicit config takes precedence; otherwise reads package.json aptre config.
func (c *Config) GetLanguages() (Languages, error) {
	if len(c.Languages) != 0 {
		return NewLanguages(c.Languages)
	}

	aptre, err := c.readAptreConfig()
	if err != nil {
		return nil, err
	}
	return NewLanguages(aptre.Languages)
}

// GetRPCLibraries returns configured RPC generators.
// Explicit config takes precedence; otherwise reads package.json aptre config.
func (c *Config) GetRPCLibraries() (RPCLibraries, error) {
	if len(c.RPCLibraries) != 0 {
		return NewRPCLibraries(c.RPCLibraries)
	}

	aptre, err := c.readAptreConfig()
	if err != nil {
		return nil, err
	}
	return NewRPCLibraries(aptre.RPCLibraries)
}

// GetRust returns the whole-graph Rust configuration from package.json.
// It returns nil when the project does not configure aptre.rust.
func (c *Config) GetRust() (*RustConfig, error) {
	// Absence leaves the project's existing per-directory generation mode selected.
	aptre, err := c.readAptreConfig()
	if err != nil {
		return nil, err
	}
	if aptre.Rust == nil {
		return nil, nil
	}

	// Apply the inventory default without mutating the parsed configuration.
	rust := *aptre.Rust
	if rust.Inventory == "" {
		rust.Inventory = DefaultRustInventory
	}
	return &rust, nil
}

// GetSchemaModule returns the import path that prefixes this project's schemas.
// The aptre.module setting in package.json takes precedence, which lets a
// project without go.mod name its schemas; otherwise it is the Go module path.
func (c *Config) GetSchemaModule() (string, error) {
	aptre, err := c.readAptreConfig()
	if err != nil {
		return "", err
	}
	if aptre.Module != "" {
		return aptre.Module, nil
	}
	return c.GetGoModule()
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
			return "", errors.Wrapf(os.ErrNotExist, "go.mod not found in %s or ancestors", projectDir)
		}
		dir = parent
	}
}
