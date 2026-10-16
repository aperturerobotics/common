package protogen

import (
	"bytes"
	"context"
	"os/exec"

	prost "github.com/aperturerobotics/go-protoc-gen-prost"
	"github.com/pkg/errors"
	"github.com/tetratelabs/wazero"
)

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
	// Reuse an initialized plugin inside the same protoc runtime.
	if h.prostWASM != nil {
		return nil // Already initialized
	}
	p, err := prost.NewProtocGenProstWithWASI(ctx, runtime)
	if err != nil {
		return errors.Wrap(err, "failed to initialize prost WASM")
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
// For protoc-gen-prost, it uses the embedded WASM module if initialized, and
// it serves protoc-gen-go-typeid in process.
func (h *NativePluginHandler) Communicate(ctx context.Context, program string, searchPath bool, input []byte) ([]byte, error) {
	// Serve the embedded generators without a child process.
	if program == "protoc-gen-prost" && h.prostWASM != nil {
		return h.prostWASM.Execute(ctx, input)
	}
	if program == TypeIDPluginName {
		return GenerateTypeIDs(input)
	}

	// Find the plugin path
	pluginPath := h.findPluginPath(program, searchPath)
	if pluginPath == "" {
		return nil, errors.Errorf("plugin not found: %s", program)
	}

	// The native process reads one request and returns one complete plugin response.
	cmd := exec.CommandContext(ctx, pluginPath)
	cmd.Stdin = bytes.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// A failed plugin response is never forwarded to protoc as generated output.
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return nil, errors.Errorf("plugin %s failed: %v: %s", program, err, stderr.String())
		}
		return nil, errors.Errorf("plugin %s failed: %v", program, err)
	}

	return stdout.Bytes(), nil
}

// findPluginPath finds the plugin binary path.
func (h *NativePluginHandler) findPluginPath(program string, searchPath bool) string {
	// Check our configured plugins first
	if h.Plugins != nil {
		switch program {
		case "protoc-gen-starpc-python":
			if h.Plugins.StarpcPython != nil {
				return h.Plugins.StarpcPython.Path
			}
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
