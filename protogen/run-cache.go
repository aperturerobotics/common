package protogen

import (
	"os"
	"path/filepath"
	"slices"
)

// RunCache records the package inventory for one generation selection.
type RunCache struct {
	// ProtocFlagsHash is the hash of the protoc flags.
	ProtocFlagsHash string `json:"protocFlagsHash"`
	// ToolVersions identifies the toolchain that generated the outputs.
	ToolVersions string `json:"toolVersions,omitempty"`
	// Packages maps package identifiers to their inputs and generated files.
	Packages map[string]*PackageInfo `json:"packages"`
}

// NewRunCache creates an empty output inventory.
func NewRunCache() *RunCache {
	return &RunCache{Packages: make(map[string]*PackageInfo)}
}

// NeedsRegeneration checks if a cached package needs regeneration.
//
// Returns true if:
// - The package is not in the cache
// - The proto file list or content hash has changed
// - The protoc flags or selected tool versions have changed
// - Force is true
func (c *RunCache) NeedsRegeneration(packageKey string, protoFiles []string, projectDir string, flagsHash string, toolVersions string, force bool) (bool, error) {
	// An explicit force request bypasses every cache identity.
	if force {
		return true, nil
	}

	// A different toolchain or protoc configuration invalidates the outputs.
	if c.ToolVersions != toolVersions || c.ProtocFlagsHash != flagsHash {
		return true, nil
	}

	// A package without a recorded identity must be generated.
	info := c.Packages[packageKey]
	if info == nil || !slices.Equal(info.ProtoFiles, protoFiles) {
		return true, nil
	}

	// Reuse outputs only if the recorded source content still matches.
	currentHash, err := hashProtoFiles(protoFiles, projectDir)
	if err != nil {
		return true, nil
	}
	return info.Hash != currentHash, nil
}

// UpdatePackage updates the cache for a package after generation.
func (c *RunCache) UpdatePackage(packageKey string, protoFiles []string, generatedFiles []string, projectDir string) error {
	// Hash the sources before replacing the previous package inventory.
	hash, err := hashProtoFiles(protoFiles, projectDir)
	if err != nil {
		return err
	}

	// Record which inputs produced these output paths.
	c.Packages[packageKey] = &PackageInfo{
		Hash:           hash,
		GeneratedFiles: generatedFiles,
		ProtoFiles:     protoFiles,
	}
	return nil
}

// CleanOrphanedPackages removes packages from the cache that are not in
// currentPackages and no longer have any proto file on disk. Packages outside
// currentPackages whose sources still exist are kept, so a run targeting a
// subset of the tree does not discard the entries of the rest.
func (c *RunCache) CleanOrphanedPackages(currentPackages map[string]struct{}, projectDir string) {
	for key, info := range c.Packages {
		if _, ok := currentPackages[key]; ok {
			continue
		}
		if info != nil && protoFilesExist(info.ProtoFiles, projectDir) {
			continue
		}
		delete(c.Packages, key)
	}
}

// DropOutsidePackages removes every package not in currentPackages. It is used
// when the protoc flags or tool versions changed: the outputs of packages that
// were not regenerated no longer match the new identity recorded in the cache.
func (c *RunCache) DropOutsidePackages(currentPackages map[string]struct{}) {
	for key := range c.Packages {
		if _, ok := currentPackages[key]; !ok {
			delete(c.Packages, key)
		}
	}
}

// protoFilesExist reports whether any of the project-relative proto files exist.
func protoFilesExist(protoFiles []string, projectDir string) bool {
	for _, f := range protoFiles {
		if _, err := os.Stat(filepath.Join(projectDir, f)); err == nil {
			return true
		}
	}
	return false
}
