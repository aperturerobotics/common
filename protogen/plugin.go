package protogen

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
