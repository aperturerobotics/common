// Package cache stores generation manifests and computes checkout-portable
// identities for generation selections, protoc flags and source packages.
package cache

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
const CacheVersion = 3

// Cache represents the protoc manifest cache.
type Cache struct {
	// Version is the cache format version.
	Version int `json:"version"`
	// Runs maps generation selections to their independent output inventories.
	Runs map[string]*RunCache `json:"runs"`
}

// NewCache creates a new empty cache.
func NewCache() *Cache {
	return &Cache{
		Version: CacheVersion,
		Runs:    make(map[string]*RunCache),
	}
}

// LoadCache loads the cache from a file.
// A missing, invalid or old manifest is a cache miss.
func LoadCache(path string) (*Cache, error) {
	// Read the manifest, treating a missing file as an empty cache.
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return NewCache(), nil
	}
	if err != nil {
		return nil, err
	}

	// Discard a manifest whose JSON cannot be decoded.
	var cache Cache
	if err := json.Unmarshal(data, &cache); err != nil {
		return NewCache(), nil
	}

	// Start fresh after a manifest format change.
	if cache.Version != CacheVersion {
		return NewCache(), nil
	}

	// Ensure callers can add runs to an empty manifest.
	if cache.Runs == nil {
		cache.Runs = make(map[string]*RunCache)
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

// GetPackageKey generates a cache key for a proto file.
// It joins the module path with the proto file's directory.
func GetPackageKey(modulePath, protoFile string) string {
	dir := filepath.Dir(protoFile)
	return filepath.Join(modulePath, dir)
}

// hashProtoFiles computes a hash of the contents of multiple proto files.
func hashProtoFiles(protoFiles []string, projectDir string) (string, error) {
	// Hash source names and contents into the package identity.
	h := sha256.New()

	// Sort files for deterministic hashing
	sorted := make([]string, len(protoFiles))
	copy(sorted, protoFiles)
	slices.Sort(sorted)

	// Read sources in sorted order without changing the caller's list.
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
