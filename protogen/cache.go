package protogen

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// CacheVersion is the current cache format version.
const CacheVersion = 2

// Cache represents the protoc manifest cache.
type Cache struct {
	// Version is the cache format version.
	Version int `json:"version"`
	// ProtocFlagsHash is the hash of the protoc flags.
	ProtocFlagsHash string `json:"protocFlagsHash"`
	// ToolVersions stores tool version strings for cache invalidation.
	ToolVersions string `json:"toolVersions,omitempty"`
	// Packages maps package identifiers to package info.
	Packages map[string]*PackageInfo `json:"packages"`
}

// PackageInfo contains cached information about a proto package.
type PackageInfo struct {
	// Hash is the content hash of all proto files in this package.
	Hash string `json:"hash"`
	// GeneratedFiles is the list of generated output files.
	GeneratedFiles []string `json:"generatedFiles"`
	// ProtoFiles is the list of source proto file paths.
	ProtoFiles []string `json:"protoFiles"`
}

// NewCache creates a new empty cache.
func NewCache() *Cache {
	return &Cache{
		Version:  CacheVersion,
		Packages: make(map[string]*PackageInfo),
	}
}

// LoadCache loads the cache from a file.
// Returns an empty cache if the file doesn't exist.
func LoadCache(path string) (*Cache, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return NewCache(), nil
	}
	if err != nil {
		return nil, err
	}

	var cache Cache
	if err := json.Unmarshal(data, &cache); err != nil {
		// If cache is corrupted, return empty cache
		return NewCache(), nil
	}

	// Check version compatibility
	if cache.Version != CacheVersion {
		return NewCache(), nil
	}

	if cache.Packages == nil {
		cache.Packages = make(map[string]*PackageInfo)
	}

	return &cache, nil
}

// Save writes the cache to a file.
func (c *Cache) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// SetProtocFlags sets the protoc flags hash.
func (c *Cache) SetProtocFlags(flags []string) {
	c.ProtocFlagsHash = hashStrings(flags)
}

// SetToolVersions sets the tool versions string.
func (c *Cache) SetToolVersions(versions string) {
	c.ToolVersions = versions
}

// NeedsRegeneration checks if a proto file needs regeneration.
// Returns true if:
// - The file is not in the cache
// - The file content hash has changed
// - The protoc flags have changed
// - Force is true
func (c *Cache) NeedsRegeneration(packageKey string, protoFiles []string, projectDir string, flagsHash string, force bool) (bool, error) {
	if force {
		return true, nil
	}

	// Check if flags changed
	if c.ProtocFlagsHash != flagsHash {
		return true, nil
	}

	info, ok := c.Packages[packageKey]
	if !ok {
		return true, nil
	}

	// Check if proto files list changed
	if !stringsEqual(info.ProtoFiles, protoFiles) {
		return true, nil
	}

	// Check content hash
	currentHash, err := hashProtoFiles(protoFiles, projectDir)
	if err != nil {
		return true, nil
	}

	return info.Hash != currentHash, nil
}

// UpdatePackage updates the cache for a package after generation.
func (c *Cache) UpdatePackage(packageKey string, protoFiles []string, generatedFiles []string, projectDir string) error {
	hash, err := hashProtoFiles(protoFiles, projectDir)
	if err != nil {
		return err
	}

	c.Packages[packageKey] = &PackageInfo{
		Hash:           hash,
		GeneratedFiles: generatedFiles,
		ProtoFiles:     protoFiles,
	}

	return nil
}

// GetPackageKey generates a cache key for a proto file.
// Uses the format: "module/path/to/dir;package_name"
func GetPackageKey(modulePath, protoFile string) string {
	dir := filepath.Dir(protoFile)
	return filepath.Join(modulePath, dir)
}

// hashProtoFiles computes a hash of the contents of multiple proto files.
func hashProtoFiles(protoFiles []string, projectDir string) (string, error) {
	h := sha256.New()

	// Sort files for deterministic hashing
	sorted := make([]string, len(protoFiles))
	copy(sorted, protoFiles)
	sort.Strings(sorted)

	for _, f := range sorted {
		path := filepath.Join(projectDir, f)
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		h.Write([]byte(f))
		h.Write(data)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// hashStrings computes a hash of a string slice.
func hashStrings(strs []string) string {
	h := sha256.New()
	for _, s := range strs {
		h.Write([]byte(s))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// stringsEqual checks if two string slices are equal.
func stringsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// CleanOrphanedPackages removes packages from the cache that no longer have proto files.
func (c *Cache) CleanOrphanedPackages(currentPackages map[string]struct{}) {
	for key := range c.Packages {
		if _, ok := currentPackages[key]; !ok {
			delete(c.Packages, key)
		}
	}
}
