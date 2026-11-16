package protogen

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
