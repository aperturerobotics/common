package protogen

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	prost "github.com/aperturerobotics/go-protoc-gen-prost"
	"github.com/tetratelabs/wazero"
)

// PluginType represents the type of protoc plugin.
type PluginType int

const (
	PluginTypeGo PluginType = iota
	PluginTypeTypeScript
	PluginTypeCpp
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
}

// Plugins holds the configured plugins for a project.
type Plugins struct {
	// GoLite is the protoc-gen-go-lite plugin.
	GoLite *Plugin
	// GoStarpc is the protoc-gen-go-starpc plugin.
	GoStarpc *Plugin
	// ESLite is the protoc-gen-es-lite plugin.
	ESLite *Plugin
	// ESStarpc is the protoc-gen-es-starpc plugin.
	ESStarpc *Plugin
	// CppStarpc is the protoc-gen-starpc-cpp plugin.
	CppStarpc *Plugin
	// RustStarpc is the protoc-gen-starpc-rust plugin.
	RustStarpc *Plugin
	// RustProst is the protoc-gen-prost plugin for Rust protobuf types.
	// This uses an embedded WASM module, no external binary required.
	RustProst *Plugin
}

// DiscoverPlugins finds and configures available plugins.
func DiscoverPlugins(cfg *Config) (*Plugins, error) {
	projectDir, err := cfg.GetProjectDir()
	if err != nil {
		return nil, err
	}

	toolsDir, err := cfg.GetToolsDir()
	if err != nil {
		return nil, err
	}
	toolsBin := filepath.Join(toolsDir, "bin")

	hasGo, err := cfg.HasGoMod()
	if err != nil {
		return nil, err
	}

	hasTS, err := cfg.HasPackageJSON()
	if err != nil {
		return nil, err
	}

	plugins := &Plugins{}

	if hasGo {
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
		}

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

		rustStarpcPath := filepath.Join(toolsBin, "protoc-gen-starpc-rust")
		if _, err := os.Stat(rustStarpcPath); err == nil {
			plugins.RustStarpc = &Plugin{
				Name:       "starpc-rust",
				BinaryName: "protoc-gen-starpc-rust",
				Path:       rustStarpcPath,
				Type:       PluginTypeRust,
				OutFlag:    "starpc-rust_out",
				Options:    map[string]string{},
			}
		}

		// protoc-gen-prost is available as embedded WASM, always enable it.
		// The WASM module is used by default; native binary path is optional fallback.
		plugins.RustProst = &Plugin{
			Name:       "prost",
			BinaryName: "protoc-gen-prost",
			Path:       "", // WASM module used by default, no native path needed
			Type:       PluginTypeRust,
			OutFlag:    "prost_out",
			Options:    map[string]string{},
		}
		// Check if native binary exists (for potential future fallback)
		prostPath := filepath.Join(toolsBin, "protoc-gen-prost")
		if _, err := os.Stat(prostPath); err == nil {
			plugins.RustProst.Path = prostPath
		} else if path, err := exec.LookPath("protoc-gen-prost"); err == nil {
			plugins.RustProst.Path = path
		}
	}

	if hasTS {
		// TypeScript plugins from node_modules
		nodeModules := filepath.Join(projectDir, "node_modules", ".bin")

		esLitePath := filepath.Join(nodeModules, "protoc-gen-es-lite")
		if _, err := os.Stat(esLitePath); err == nil {
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
		}

		esStarpcPath := filepath.Join(nodeModules, "protoc-gen-es-starpc")
		if _, err := os.Stat(esStarpcPath); err == nil {
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

	return plugins, nil
}

// GetProtocArgs returns the protoc arguments for all configured plugins.
// Note: We don't pass --plugin=<path> because the WASI protoc can't access
// host binaries. Instead, the PluginHandler intercepts plugin calls by name.
func (p *Plugins) GetProtocArgs(outDir string) []string {
	var args []string

	// C++ output (built-in to protoc)
	args = append(args, fmt.Sprintf("--cpp_out=%s", outDir))

	// Go plugins
	if p.GoLite != nil {
		args = append(args, fmt.Sprintf("--%s=%s", p.GoLite.OutFlag, outDir))
		for k, v := range p.GoLite.Options {
			args = append(args, fmt.Sprintf("--%s_opt=%s=%s", p.GoLite.Name, k, v))
		}
	}

	if p.GoStarpc != nil {
		args = append(args, fmt.Sprintf("--%s=%s", p.GoStarpc.OutFlag, outDir))
	}

	// TypeScript plugins
	if p.ESLite != nil {
		args = append(args, fmt.Sprintf("--%s=%s", p.ESLite.OutFlag, outDir))
		for k, v := range p.ESLite.Options {
			args = append(args, fmt.Sprintf("--%s_opt=%s=%s", p.ESLite.Name, k, v))
		}
	}

	if p.ESStarpc != nil {
		args = append(args, fmt.Sprintf("--%s=%s", p.ESStarpc.OutFlag, outDir))
		for k, v := range p.ESStarpc.Options {
			args = append(args, fmt.Sprintf("--%s_opt=%s=%s", p.ESStarpc.Name, k, v))
		}
	}

	// C++ starpc plugin
	if p.CppStarpc != nil {
		args = append(args, fmt.Sprintf("--%s=%s", p.CppStarpc.OutFlag, outDir))
	}

	// Rust prost plugin (generates *.pb.rs message types)
	if p.RustProst != nil {
		args = append(args, fmt.Sprintf("--%s=%s", p.RustProst.OutFlag, outDir))
	}

	// Rust starpc plugin (generates *_srpc.pb.rs service stubs)
	if p.RustStarpc != nil {
		args = append(args, fmt.Sprintf("--%s=%s", p.RustStarpc.OutFlag, outDir))
	}

	return args
}

// HasGoPlugins returns true if Go plugins are configured.
func (p *Plugins) HasGoPlugins() bool {
	return p.GoLite != nil || p.GoStarpc != nil
}

// HasTSPlugins returns true if TypeScript plugins are configured.
func (p *Plugins) HasTSPlugins() bool {
	return p.ESLite != nil || p.ESStarpc != nil
}

// NativePluginHandler implements go-protoc-wasi's PluginHandler interface.
// It spawns native plugin processes and handles IPC.
// For protoc-gen-prost, it uses the embedded WASM module instead of a native binary.
type NativePluginHandler struct {
	// Plugins is the configured plugins.
	Plugins *Plugins
	// Verbose enables verbose output.
	Verbose bool
	// prostWASM is the prost WASM plugin instance (lazily initialized).
	prostWASM *prost.ProtocGenProst
}

// NewNativePluginHandler creates a new NativePluginHandler.
func NewNativePluginHandler(plugins *Plugins, verbose bool) *NativePluginHandler {
	return &NativePluginHandler{
		Plugins: plugins,
		Verbose: verbose,
	}
}

// InitProstWASM initializes the prost WASM plugin with the given wazero runtime.
// The runtime must already have WASI instantiated (e.g., by protoc).
// This should be called after protoc.Init() and before running protoc.
func (h *NativePluginHandler) InitProstWASM(ctx context.Context, runtime wazero.Runtime) error {
	if h.prostWASM != nil {
		return nil // Already initialized
	}
	p, err := prost.NewProtocGenProstWithWASI(ctx, runtime)
	if err != nil {
		return fmt.Errorf("failed to initialize prost WASM: %w", err)
	}
	h.prostWASM = p
	return nil
}

// CloseProstWASM closes the prost WASM plugin if initialized.
func (h *NativePluginHandler) CloseProstWASM(ctx context.Context) error {
	if h.prostWASM != nil {
		err := h.prostWASM.Close(ctx)
		h.prostWASM = nil
		return err
	}
	return nil
}

// Communicate implements the PluginHandler interface.
// It spawns a plugin process, sends the CodeGeneratorRequest via stdin,
// and returns the CodeGeneratorResponse from stdout.
// For protoc-gen-prost, it uses the embedded WASM module if initialized.
func (h *NativePluginHandler) Communicate(ctx context.Context, program string, searchPath bool, input []byte) ([]byte, error) {
	// Use WASM prost plugin if available
	if program == "protoc-gen-prost" && h.prostWASM != nil {
		return h.prostWASM.Execute(ctx, input)
	}

	// Find the plugin path
	pluginPath := h.findPluginPath(program, searchPath)
	if pluginPath == "" {
		return nil, fmt.Errorf("plugin not found: %s", program)
	}

	cmd := exec.CommandContext(ctx, pluginPath)
	cmd.Stdin = bytes.NewReader(input)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("plugin %s failed: %v: %s", program, err, stderr.String())
		}
		return nil, fmt.Errorf("plugin %s failed: %v", program, err)
	}

	return stdout.Bytes(), nil
}

// findPluginPath finds the plugin binary path.
func (h *NativePluginHandler) findPluginPath(program string, searchPath bool) string {
	// Check our configured plugins first
	if h.Plugins != nil {
		switch program {
		case "protoc-gen-go-lite":
			if h.Plugins.GoLite != nil {
				return h.Plugins.GoLite.Path
			}
		case "protoc-gen-go-starpc":
			if h.Plugins.GoStarpc != nil {
				return h.Plugins.GoStarpc.Path
			}
		case "protoc-gen-es-lite":
			if h.Plugins.ESLite != nil {
				return h.Plugins.ESLite.Path
			}
		case "protoc-gen-es-starpc":
			if h.Plugins.ESStarpc != nil {
				return h.Plugins.ESStarpc.Path
			}
		case "protoc-gen-starpc-cpp":
			if h.Plugins.CppStarpc != nil {
				return h.Plugins.CppStarpc.Path
			}
		case "protoc-gen-starpc-rust":
			if h.Plugins.RustStarpc != nil {
				return h.Plugins.RustStarpc.Path
			}
		case "protoc-gen-prost":
			if h.Plugins.RustProst != nil {
				return h.Plugins.RustProst.Path
			}
		}
	}

	// Fall back to PATH search if allowed
	if searchPath {
		if path, err := exec.LookPath(program); err == nil {
			return path
		}
	}

	return ""
}
