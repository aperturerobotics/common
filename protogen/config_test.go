package protogen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestConfigHasGoModOutsideModule reports absence without rejecting a Rust-only project.
func TestConfigHasGoModOutsideModule(t *testing.T) {
	// A fresh project has no Go module in its parent chain.
	cfg := NewConfig()
	cfg.ProjectDir = t.TempDir()

	// Wrapped module-discovery absence remains an ordinary false result.
	hasGo, err := cfg.HasGoMod()
	if err != nil {
		t.Fatal(err)
	}
	if hasGo {
		t.Fatal("temporary project unexpectedly has a Go module")
	}
}

func TestConfigGetGoModuleFromSubdir(t *testing.T) {
	// Build an isolated project with explicit package configuration.
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "net")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}

	// The module root remains above the selected schema directory.
	goMod := []byte("module github.com/example/app\n\ngo 1.25.0\n")
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), goMod, 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	// Resolve settings through the public project configuration.
	cfg := NewConfig()
	cfg.ProjectDir = projectDir

	// Module discovery returns the owning root from a nested directory.
	moduleDir, err := cfg.GetModuleDir()
	if err != nil {
		t.Fatalf("get module dir: %v", err)
	}
	if moduleDir != tmpDir {
		t.Fatalf("expected module dir %q, got %q", tmpDir, moduleDir)
	}

	// Schema identity includes the path below the owning module.
	modulePath, err := cfg.GetGoModule()
	if err != nil {
		t.Fatalf("get go module: %v", err)
	}
	if modulePath != "github.com/example/app/net" {
		t.Fatalf("expected module path %q, got %q", "github.com/example/app/net", modulePath)
	}

	// The nested project still has access to its parent Go module.
	hasGoMod, err := cfg.HasGoMod()
	if err != nil {
		t.Fatalf("has go mod: %v", err)
	}
	if !hasGoMod {
		t.Fatal("expected has go mod to be true")
	}
}

func TestConfigGetTsImportBoundariesFromPackageJSON(t *testing.T) {
	// Build an isolated project with explicit package configuration.
	tmpDir := t.TempDir()
	packageJSON := []byte(`{
  "name": "example-app",
  "aptre": {
    "tsImportBoundaries": ["bldr", "db", "net"]
  }
}`)
	if err := os.WriteFile(filepath.Join(tmpDir, "package.json"), packageJSON, 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}

	// Resolve settings through the public project configuration.
	cfg := NewConfig()
	cfg.ProjectDir = tmpDir

	// Preserve the package boundary order supplied by the project.
	boundaries, err := cfg.GetTsImportBoundaries()
	if err != nil {
		t.Fatalf("get ts import boundaries: %v", err)
	}
	if len(boundaries) != 3 {
		t.Fatalf("expected 3 boundaries, got %d", len(boundaries))
	}
	if boundaries[0] != "bldr" || boundaries[1] != "db" || boundaries[2] != "net" {
		t.Fatalf("unexpected boundaries: %v", boundaries)
	}
}

func TestConfigGetLanguagesFromPackageJSON(t *testing.T) {
	// Build an isolated project with explicit package configuration.
	tmpDir := t.TempDir()
	packageJSON := []byte(`{
  "name": "example-app",
  "aptre": {
    "languages": ["go", "rust"]
  }
}`)
	if err := os.WriteFile(filepath.Join(tmpDir, "package.json"), packageJSON, 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}

	// Resolve settings through the public project configuration.
	cfg := NewConfig()
	cfg.ProjectDir = tmpDir

	// Resolve the effective language set using explicit settings before defaults.
	langs, err := cfg.GetLanguages()
	if err != nil {
		t.Fatalf("get languages: %v", err)
	}
	if !langs.Has(LanguageGo) {
		t.Fatal("expected go language to be enabled")
	}
	if !langs.Has(LanguageRust) {
		t.Fatal("expected rust language to be enabled")
	}
	if langs.Has(LanguageCpp) {
		t.Fatal("expected cpp language to be disabled")
	}
	if langs.Has(LanguageTypeScript) {
		t.Fatal("expected ts language to be disabled")
	}
}

func TestConfigGetLanguagesExplicitTakesPrecedence(t *testing.T) {
	// Build an isolated project with explicit package configuration.
	tmpDir := t.TempDir()
	packageJSON := []byte(`{
  "name": "example-app",
  "aptre": {
    "languages": ["rust"]
  }
}`)
	if err := os.WriteFile(filepath.Join(tmpDir, "package.json"), packageJSON, 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}

	// Resolve settings through the public project configuration.
	cfg := NewConfig()
	cfg.ProjectDir = tmpDir
	cfg.Languages = []string{"go"}

	// Resolve the effective language set using explicit settings before defaults.
	langs, err := cfg.GetLanguages()
	if err != nil {
		t.Fatalf("get languages: %v", err)
	}
	if !langs.Has(LanguageGo) {
		t.Fatal("expected explicit go language to be enabled")
	}
	if langs.Has(LanguageRust) {
		t.Fatal("expected package.json rust language to be ignored")
	}
}

func TestConfigGetRPCLibrariesFromPackageJSON(t *testing.T) {
	// Build an isolated project with explicit package configuration.
	tmpDir := t.TempDir()
	packageJSON := []byte(`{
  "name": "example-app",
  "aptre": {
    "rpc": ["none"]
  }
}`)
	if err := os.WriteFile(filepath.Join(tmpDir, "package.json"), packageJSON, 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}

	// Resolve settings through the public project configuration.
	cfg := NewConfig()
	cfg.ProjectDir = tmpDir

	// Resolve RPC selection independently of message generation.
	rpcs, err := cfg.GetRPCLibraries()
	if err != nil {
		t.Fatalf("get RPC libraries: %v", err)
	}
	if rpcs.Has(RPCLibraryStarpc) {
		t.Fatal("expected starpc RPC generation to be disabled")
	}
}

func TestConfigGetRPCLibrariesExplicitTakesPrecedence(t *testing.T) {
	// Build an isolated project with explicit package configuration.
	tmpDir := t.TempDir()
	packageJSON := []byte(`{
  "name": "example-app",
  "aptre": {
    "rpc": ["none"]
  }
}`)
	if err := os.WriteFile(filepath.Join(tmpDir, "package.json"), packageJSON, 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}

	// Resolve settings through the public project configuration.
	cfg := NewConfig()
	cfg.ProjectDir = tmpDir
	cfg.RPCLibraries = []string{"starpc"}

	// Resolve RPC selection independently of message generation.
	rpcs, err := cfg.GetRPCLibraries()
	if err != nil {
		t.Fatalf("get RPC libraries: %v", err)
	}
	if !rpcs.Has(RPCLibraryStarpc) {
		t.Fatal("expected explicit starpc RPC generation to be enabled")
	}
}

func TestConfigGetRPCLibrariesDefaultStarpc(t *testing.T) {
	// Resolve settings through the public project configuration.
	cfg := NewConfig()
	cfg.ProjectDir = t.TempDir()

	// Resolve RPC selection independently of message generation.
	rpcs, err := cfg.GetRPCLibraries()
	if err != nil {
		t.Fatalf("get RPC libraries: %v", err)
	}
	if !rpcs.Has(RPCLibraryStarpc) {
		t.Fatal("expected starpc RPC generation to be enabled by default")
	}
}

func TestConfigGetRPCLibrariesFalseAlias(t *testing.T) {
	// Resolve settings through the public project configuration.
	cfg := NewConfig()
	cfg.ProjectDir = t.TempDir()
	cfg.RPCLibraries = []string{"false"}

	// Resolve RPC selection independently of message generation.
	rpcs, err := cfg.GetRPCLibraries()
	if err != nil {
		t.Fatalf("get RPC libraries: %v", err)
	}
	if rpcs.Has(RPCLibraryStarpc) {
		t.Fatal("expected false to disable RPC generation")
	}
}

func TestConfigGetLanguagesDefaultAll(t *testing.T) {
	// Resolve settings through the public project configuration.
	cfg := NewConfig()
	cfg.ProjectDir = t.TempDir()

	// Resolve the effective language set using explicit settings before defaults.
	langs, err := cfg.GetLanguages()
	if err != nil {
		t.Fatalf("get languages: %v", err)
	}
	for _, lang := range []Language{LanguageGo, LanguageTypeScript, LanguageCpp, LanguageRust} {
		if !langs.Has(lang) {
			t.Fatalf("expected language %q to be enabled by default", lang)
		}
	}
	for _, lang := range []Language{LanguageCSharp, LanguagePython} {
		if langs.Has(lang) {
			t.Fatalf("expected opt-in language %q to be disabled by default", lang)
		}
	}
}

func TestConfigGetLanguagesUnknown(t *testing.T) {
	// Resolve settings through the public project configuration.
	cfg := NewConfig()
	cfg.ProjectDir = t.TempDir()
	cfg.Languages = []string{"go", "kotlin"}

	// Unsupported language names must produce a configuration error.
	if _, err := cfg.GetLanguages(); err == nil {
		t.Fatal("expected unknown language error")
	}
}

func TestConfigGetRPCLibrariesStarpcPython(t *testing.T) {
	// Resolve settings through the public project configuration.
	cfg := NewConfig()
	cfg.RPCLibraries = []string{"starpc-python"}

	// Resolve RPC selection independently of message generation.
	rpcs, err := cfg.GetRPCLibraries()
	if err != nil {
		t.Fatalf("get RPC libraries: %v", err)
	}
	if !rpcs.Has(RPCLibraryStarpcPython) {
		t.Fatal("expected starpc-python RPC generation to be enabled")
	}
	if rpcs.Has(RPCLibraryStarpc) {
		t.Fatal("starpc-python must not imply starpc")
	}
}

func TestConfigGetRustFromPackageJSON(t *testing.T) {
	// Build an isolated project with explicit package configuration.
	tmpDir := t.TempDir()
	packageJSON := []byte(`{
  "aptre": {
    "module": "github.com/example/app",
    "languages": ["rust"],
    "rust": {
      "prostOptions": ["enable_type_names", "btree_map=."],
      "exclude": ["vendor/skip.proto"],
      "descriptorSet": "wire/descriptors.bin",
      "moduleFile": "wire/messages.rs"
    }
  }
}`)
	if err := os.WriteFile(filepath.Join(tmpDir, "package.json"), packageJSON, 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}

	// Resolve settings through the public project configuration.
	cfg := NewConfig()
	cfg.ProjectDir = tmpDir

	// Only an explicit Rust section enables whole-graph generation.
	rust, err := cfg.GetRust()
	if err != nil {
		t.Fatalf("get rust: %v", err)
	}
	if rust == nil {
		t.Fatal("expected rust config")
	}
	if len(rust.ProstOptions) != 2 || rust.ProstOptions[1] != "btree_map=." {
		t.Fatalf("unexpected prost options %v", rust.ProstOptions)
	}
	if rust.DescriptorSet != "wire/descriptors.bin" || rust.ModuleFile != "wire/messages.rs" {
		t.Fatalf("unexpected outputs %q, %q", rust.DescriptorSet, rust.ModuleFile)
	}
	if rust.Inventory != DefaultRustInventory {
		t.Fatalf("expected default inventory %q, got %q", DefaultRustInventory, rust.Inventory)
	}

	// The module setting names schemas for a project without go.mod.
	modulePath, err := cfg.GetSchemaModule()
	if err != nil {
		t.Fatalf("get schema module: %v", err)
	}
	if modulePath != "github.com/example/app" {
		t.Fatalf("unexpected schema module %q", modulePath)
	}
}

func TestConfigGetRustUnconfigured(t *testing.T) {
	// Resolve settings through the public project configuration.
	cfg := NewConfig()
	cfg.ProjectDir = t.TempDir()

	// Only an explicit Rust section enables whole-graph generation.
	rust, err := cfg.GetRust()
	if err != nil {
		t.Fatalf("get rust: %v", err)
	}
	if rust != nil {
		t.Fatalf("expected nil rust config, got %+v", rust)
	}
}

// TestConfigRejectsMalformedAptreTypes names the field of each wrongly typed entry.
func TestConfigRejectsMalformedAptreTypes(t *testing.T) {
	// Every case has one field of the wrong JSON type.
	cases := []struct {
		name string
		json string
		want string
	}{
		{"aptre scalar", `{"aptre": true}`, "aptre"},
		{"module number", `{"aptre": {"module": 7}}`, "module"},
		{"languages object", `{"aptre": {"languages": {}}}`, "languages"},
		{"language number", `{"aptre": {"languages": ["rust", 1]}}`, "languages"},
		{"rust scalar", `{"aptre": {"rust": "yes"}}`, "aptre.rust"},
		{"rust option number", `{"aptre": {"rust": {"prostOptions": [2]}}}`, "prostOptions"},
		{"rust output array", `{"aptre": {"rust": {"moduleFile": []}}}`, "moduleFile"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Write the manifest for an isolated project.
			tmpDir := t.TempDir()
			if err := os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(tc.json), 0o644); err != nil {
				t.Fatalf("write package.json: %v", err)
			}
			cfg := NewConfig()
			cfg.ProjectDir = tmpDir

			// The error identifies the field instead of silently using defaults.
			_, err := cfg.GetRust()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error naming %q, got %v", tc.want, err)
			}
		})
	}
}

// TestConfigAcceptsNullAptreSections treats null sections as absent.
func TestConfigAcceptsNullAptreSections(t *testing.T) {
	// A null Rust section and null lists keep every default.
	tmpDir := t.TempDir()
	packageJSON := []byte(`{"aptre": {"module": null, "languages": null, "rust": null}}`)
	if err := os.WriteFile(filepath.Join(tmpDir, "package.json"), packageJSON, 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}
	cfg := NewConfig()
	cfg.ProjectDir = tmpDir

	// Null does not enable whole-graph generation.
	rust, err := cfg.GetRust()
	if err != nil {
		t.Fatalf("get rust: %v", err)
	}
	if rust != nil {
		t.Fatalf("expected nil rust config, got %+v", rust)
	}
}
