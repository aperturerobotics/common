package protogen

import (
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
)

// PluginType represents the type of protoc plugin.
type PluginType int

const (
	// PluginTypeGo generates Go declarations.
	PluginTypeGo PluginType = iota
	// PluginTypePython generates Python declarations.
	PluginTypePython
	// PluginTypeTypeScript generates TypeScript declarations.
	PluginTypeTypeScript
	// PluginTypeCpp generates C++ declarations.
	PluginTypeCpp
	// PluginTypeRust generates Rust declarations.
	PluginTypeRust
)

// Plugin represents a protoc plugin configuration.
type Plugin struct {
	// Name is the plugin name (e.g., "go-lite", "es-lite").
	Name string
	// BinaryName is the executable name (e.g., "protoc-gen-go-lite").
	BinaryName string
	// Path is the full path to the plugin binary.
	Path string
	// Type is the plugin type.
	Type PluginType
	// OutFlag is the output flag name (e.g., "go-lite_out").
	OutFlag string
	// Options are the plugin options.
	Options map[string]string
	// Flags are plugin options forwarded verbatim in order, for options that
	// repeat or take no value.
	Flags []string
}

// Plugins holds the configured plugins for a project.
type Plugins struct {
	// Languages contains the enabled output languages.
	Languages Languages
	// RPCLibraries contains the enabled RPC stub generators.
	RPCLibraries RPCLibraries
	// GoLite is the protoc-gen-go-lite plugin.
	GoLite *Plugin
	// GoStarpc is the protoc-gen-go-starpc plugin.
	GoStarpc *Plugin
	// GoTypeID is the protoc-gen-go-typeid plugin, served in process by
	// GenerateTypeIDs.
	GoTypeID *Plugin
	// ESLite is the protoc-gen-es-lite plugin.
	ESLite *Plugin
	// ESStarpc is the protoc-gen-es-starpc plugin.
	ESStarpc *Plugin
	// CppStarpc is the protoc-gen-starpc-cpp plugin.
	CppStarpc *Plugin
	// RustStarpc is the protoc-gen-starpc-rust plugin.
	RustStarpc *Plugin
	// StarpcPython is the protoc-gen-starpc-python plugin.
	StarpcPython *Plugin
	// RustProst is the protoc-gen-prost plugin for Rust protobuf types.
	// This uses an embedded WASM module, no external binary required.
	RustProst *Plugin
}

// discoverNodePlugin finds an installed executable or the package's own named binary.
func discoverNodePlugin(projectDir, binaryName string) string {
	// Installed package links take precedence over development sources.
	installed := filepath.Join(projectDir, "node_modules", ".bin", binaryName)
	if info, err := os.Stat(installed); err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0 {
		return installed
	}

	// A package can declare one executable or a map of named executables.
	data, err := os.ReadFile(filepath.Join(projectDir, "package.json"))
	if err != nil {
		return ""
	}
	pkg, err := fastjson.ParseBytes(data)
	if err != nil {
		return ""
	}

	// Select this plugin from either of package.json's supported bin representations.
	bin := pkg.Get("bin")
	if bin == nil {
		return ""
	}
	if bin.Type() == fastjson.TypeObject {
		bin = bin.Get(binaryName)
	}
	if bin == nil {
		return ""
	}
	rel, err := bin.StringBytes()
	if err != nil {
		return ""
	}

	// Resolve package-relative paths and require an executable regular file.
	path := string(rel)
	if !filepath.IsAbs(path) {
		path = filepath.Join(projectDir, path)
	}
	if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0 {
		return path
	}
	return ""
}

// DiscoverPlugins finds and configures available plugins.
func DiscoverPlugins(cfg *Config) (*Plugins, error) {
	// Resolve the package manifest's location.
	projectDir, err := cfg.GetProjectDir()
	if err != nil {
		return nil, err
	}

	// Built plugin binaries live in the project's configured tools directory.
	toolsDir, err := cfg.GetToolsDir()
	if err != nil {
		return nil, err
	}
	toolsBin := filepath.Join(toolsDir, "bin")

	// Go module presence controls generators whose inputs come from Go dependencies.
	hasGo, err := cfg.HasGoMod()
	if err != nil {
		return nil, err
	}

	// A package manifest makes Node plugin discovery available.
	hasTS, err := cfg.HasPackageJSON()
	if err != nil {
		return nil, err
	}

	// Resolve explicit language and service selections before finding their tools.
	langs, err := cfg.GetLanguages()
	if err != nil {
		return nil, err
	}
	rpcs, err := cfg.GetRPCLibraries()
	if err != nil {
		return nil, err
	}

	// Preserve the selection even when an optional plugin has not been built yet.
	plugins := &Plugins{Languages: langs, RPCLibraries: rpcs}

	// Go message and service generation use the same module's tools.
	if hasGo && langs.Has(LanguageGo) {
		// Go plugins from tools bin
		goLitePath := filepath.Join(toolsBin, "protoc-gen-go-lite")
		if _, err := os.Stat(goLitePath); err == nil {
			plugins.GoLite = &Plugin{
				Name:       "go-lite",
				BinaryName: "protoc-gen-go-lite",
				Path:       goLitePath,
				Type:       PluginTypeGo,
				OutFlag:    "go-lite_out",
				Options: map[string]string{
					"features": cfg.GoLiteFeatures,
				},
			}

			// Type IDs extend the generated messages, so they follow go-lite.
			plugins.GoTypeID = &Plugin{
				Name:       "go-typeid",
				BinaryName: TypeIDPluginName,
				Type:       PluginTypeGo,
				OutFlag:    "go-typeid_out",
				Options:    map[string]string{},
			}
		}

		if rpcs.Has(RPCLibraryStarpc) {
			goStarpcPath := filepath.Join(toolsBin, "protoc-gen-go-starpc")
			if _, err := os.Stat(goStarpcPath); err == nil {
				plugins.GoStarpc = &Plugin{
					Name:       "go-starpc",
					BinaryName: "protoc-gen-go-starpc",
					Path:       goStarpcPath,
					Type:       PluginTypeGo,
					OutFlag:    "go-starpc_out",
					Options:    map[string]string{},
				}
			}
		}
	}

	// Python's explicitly selected service plugin must exist in tools or the virtual environment.
	if langs.Has(LanguagePython) && rpcs.Has(RPCLibraryStarpcPython) {
		binaryName := "protoc-gen-starpc-python"
		candidates := []string{
			filepath.Join(toolsBin, binaryName),
			filepath.Join(projectDir, ".venv", "bin", binaryName),
			filepath.Join(projectDir, ".venv", "Scripts", binaryName+".exe"),
		}
		starpcPythonPath := ""
		for _, candidate := range candidates {
			if info, statErr := os.Stat(candidate); statErr == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0 {
				starpcPythonPath = candidate
				break
			}
		}
		if starpcPythonPath == "" {
			return nil, errors.Errorf("starpc-python selected but plugin is unavailable; checked %s; run `uv sync --all-packages`", strings.Join(candidates, ", "))
		}
		plugins.StarpcPython = &Plugin{Name: "starpc-python", BinaryName: binaryName, Path: starpcPythonPath, Type: PluginTypePython, OutFlag: "starpc-python_out", Options: map[string]string{}}
	}

	// The C++ service generator is supplied by Go tool preparation.
	if hasGo && langs.Has(LanguageCpp) && rpcs.Has(RPCLibraryStarpc) {
		cppStarpcPath := filepath.Join(toolsBin, "protoc-gen-starpc-cpp")
		if _, err := os.Stat(cppStarpcPath); err == nil {
			plugins.CppStarpc = &Plugin{
				Name:       "starpc-cpp",
				BinaryName: "protoc-gen-starpc-cpp",
				Path:       cppStarpcPath,
				Type:       PluginTypeCpp,
				OutFlag:    "starpc-cpp_out",
				Options:    map[string]string{},
			}
		}
	}

	// Per-directory Rust generation retains its existing Go-module selection.
	if hasGo && langs.Has(LanguageRust) {
		plugins.RustStarpc, plugins.RustProst = discoverRustPlugins(toolsBin, rpcs)
	}

	// Node package binaries supply TypeScript message and service generators.
	if hasTS && langs.Has(LanguageTypeScript) {
		// TypeScript plugins from node_modules
		esLitePath := discoverNodePlugin(projectDir, "protoc-gen-es-lite")
		if esLitePath != "" {
			plugins.ESLite = &Plugin{
				Name:       "es-lite",
				BinaryName: "protoc-gen-es-lite",
				Path:       esLitePath,
				Type:       PluginTypeTypeScript,
				OutFlag:    "es-lite_out",
				Options: map[string]string{
					"target":     "ts",
					"ts_nocheck": "false",
				},
			}
			typedDecode, err := cfg.GetTsTypedDecode()
			if err != nil {
				return nil, err
			}
			if typedDecode {
				plugins.ESLite.Options["typed_decode"] = "true"
			}
		}

		if rpcs.Has(RPCLibraryStarpc) {
			esStarpcPath := discoverNodePlugin(projectDir, "protoc-gen-es-starpc")
			if esStarpcPath != "" {
				plugins.ESStarpc = &Plugin{
					Name:       "es-starpc",
					BinaryName: "protoc-gen-es-starpc",
					Path:       esStarpcPath,
					Type:       PluginTypeTypeScript,
					OutFlag:    "es-starpc_out",
					Options: map[string]string{
						"target":     "ts",
						"ts_nocheck": "false",
					},
				}
			}
		}
	}

	return plugins, nil
}

// discoverRustPlugins finds the Rust StarPC plugin, when selected and built,
// and configures the embedded prost plugin.
func discoverRustPlugins(toolsBin string, rpcs RPCLibraries) (starpc, prost *Plugin) {
	// Services require the native plugin only when StarPC is selected.
	if rpcs.Has(RPCLibraryStarpc) {
		rustStarpcPath := filepath.Join(toolsBin, "protoc-gen-starpc-rust")
		if _, err := os.Stat(rustStarpcPath); err == nil {
			starpc = &Plugin{
				Name:       "starpc-rust",
				BinaryName: "protoc-gen-starpc-rust",
				Path:       rustStarpcPath,
				Type:       PluginTypeRust,
				OutFlag:    "starpc-rust_out",
				Options:    map[string]string{},
			}
		}
	}

	// protoc-gen-prost is available as embedded WASM, always enable it.
	// The WASM module is used by default; native binary path is optional fallback.
	prost = &Plugin{
		Name:       "prost",
		BinaryName: "protoc-gen-prost",
		Path:       "", // WASM module used by default, no native path needed
		Type:       PluginTypeRust,
		OutFlag:    "prost_out",
		Options:    map[string]string{},
	}

	// Keep a native path available to callers that invoke the handler without WASM initialization.
	prostPath := filepath.Join(toolsBin, "protoc-gen-prost")
	if _, err := os.Stat(prostPath); err == nil {
		prost.Path = prostPath
		return starpc, prost
	}
	if path, err := exec.LookPath("protoc-gen-prost"); err == nil {
		prost.Path = path
	}
	return starpc, prost
}

// sortedPluginOpts returns sorted --{name}_opt=k=v args for deterministic output.
func sortedPluginOpts(p *Plugin) []string {
	keys := slices.Sorted(maps.Keys(p.Options))
	args := make([]string, 0, len(keys))
	for _, k := range keys {
		args = append(args, "--"+p.Name+"_opt="+k+"="+p.Options[k])
	}
	return args
}

// pluginFlagArgs returns the verbatim --{name}_opt=flag args in order.
func pluginFlagArgs(p *Plugin) []string {
	args := make([]string, 0, len(p.Flags))
	for _, flag := range p.Flags {
		args = append(args, "--"+p.Name+"_opt="+flag)
	}
	return args
}

// GetProtocArgs returns deterministic protoc arguments for configured outputs.
func (p *Plugins) GetProtocArgs(outDir, csharpOutDir string) []string {
	// C++ output (built-in to protoc)
	var args []string
	if p.Languages.Has(LanguageCpp) {
		args = append(args, "--cpp_out="+outDir)
	}

	// C# output (built-in to protoc)
	if p.Languages.Has(LanguageCSharp) {
		args = append(args, "--csharp_out="+csharpOutDir)
	}

	// Python output (built-in to protoc)
	if p.Languages.Has(LanguagePython) {
		args = append(args, "--python_out="+outDir)
		args = append(args, "--pyi_out="+outDir)
	}

	// Python StarPC RPC plugin
	if p.StarpcPython != nil {
		args = append(args, "--"+p.StarpcPython.OutFlag+"="+outDir)
	}

	// Go plugins
	if p.GoLite != nil {
		args = append(args, "--"+p.GoLite.OutFlag+"="+outDir)
		args = append(args, sortedPluginOpts(p.GoLite)...)
	}

	// Go service declarations accompany the selected message output.
	if p.GoStarpc != nil {
		args = append(args, "--"+p.GoStarpc.OutFlag+"="+outDir)
	}

	// Go type IDs accompany the selected message output.
	if p.GoTypeID != nil {
		args = append(args, "--"+p.GoTypeID.OutFlag+"="+outDir)
	}

	// TypeScript plugins
	if p.ESLite != nil {
		args = append(args, "--"+p.ESLite.OutFlag+"="+outDir)
		args = append(args, sortedPluginOpts(p.ESLite)...)
	}

	// TypeScript service declarations retain their own plugin options.
	if p.ESStarpc != nil {
		args = append(args, "--"+p.ESStarpc.OutFlag+"="+outDir)
		args = append(args, sortedPluginOpts(p.ESStarpc)...)
	}

	// C++ starpc plugin
	if p.CppStarpc != nil {
		args = append(args, "--"+p.CppStarpc.OutFlag+"="+outDir)
	}

	// Rust prost plugin (generates *.pb.rs message types)
	if p.RustProst != nil {
		args = append(args, "--"+p.RustProst.OutFlag+"="+outDir)
		args = append(args, pluginFlagArgs(p.RustProst)...)
	}

	// Rust starpc plugin (generates *_srpc.pb.rs service stubs)
	if p.RustStarpc != nil {
		args = append(args, "--"+p.RustStarpc.OutFlag+"="+outDir)
		args = append(args, pluginFlagArgs(p.RustStarpc)...)
	}

	return args
}
