package protogen

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

// SetProtocFlags sets the protoc flags hash, keyed on rootDir-relative paths.
func (c *Cache) SetProtocFlags(flags []string, rootDir string) {
	c.ProtocFlagsHash = HashProtocFlags(flags, rootDir)
}

// SetToolVersions sets the tool versions string.
func (c *Cache) SetToolVersions(versions string) {
	c.ToolVersions = versions
}

// NeedsRegeneration checks if a cached package needs regeneration.
// Returns true if:
// - The package is not in the cache
// - The proto file list or content hash has changed
// - The protoc flags or selected tool versions have changed
// - Force is true
func (c *Cache) NeedsRegeneration(packageKey string, protoFiles []string, projectDir string, flagsHash string, toolVersions string, force bool) (bool, error) {
	if force {
		return true, nil
	}

	if c.ToolVersions != toolVersions {
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
	slices.Sort(sorted)

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

// HashProtocFlags computes a checkout-portable hash of the protoc flags.
// Absolute path prefixes under rootDir are rewritten to rootDir-relative form
// so two checkouts of identical content at different absolute paths hash the
// same, while flag semantics (enabled plugins, plugin options, output layout)
// still change the hash. rootDir must be the absolute Go module root.
func HashProtocFlags(flags []string, rootDir string) string {
	h := sha256.New()
	for _, f := range flags {
		h.Write([]byte(relativizeProtocFlag(f, rootDir)))
		// NUL separates flags so distinct slices cannot collide.
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// relativizeProtocFlag rewrites the rootDir absolute prefix inside a single
// protoc flag to rootDir-relative form. The flag may be a bare path or a
// "--name=<value>" pair; only the value's path portion is rewritten, and
// non-path or out-of-tree values pass through unchanged.
func relativizeProtocFlag(flag, rootDir string) string {
	if key, value, ok := strings.Cut(flag, "="); ok {
		return key + "=" + relativizePath(value, rootDir)
	}
	return relativizePath(flag, rootDir)
}

// relativizePath returns p relative to rootDir when p is an absolute path
// inside rootDir; otherwise it returns p unchanged. The result uses forward
// slashes so the hash is identical across operating systems.
func relativizePath(p, rootDir string) string {
	if rootDir == "" || !filepath.IsAbs(p) {
		return p
	}
	rel, err := filepath.Rel(rootDir, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return p
	}
	return filepath.ToSlash(rel)
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
