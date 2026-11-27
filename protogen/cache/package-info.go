package cache

// PackageInfo contains cached information about a proto package.
type PackageInfo struct {
	// Hash is the content hash of all proto files in this package.
	Hash string `json:"hash"`
	// GeneratedFiles is the list of generated output files.
	GeneratedFiles []string `json:"generatedFiles"`
	// ProtoFiles is the list of source proto file paths.
	ProtoFiles []string `json:"protoFiles"`
}
