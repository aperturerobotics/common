// Package golangcilint tracks the inputs to custom golangci-lint builds.
package golangcilint

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// Fingerprint hashes a custom configuration and the contents of local plugins.
// Versioned plugin inputs are pinned in the configuration; local plugins include
// source, module files, and embedded assets. External local replacements and
// environment-specific build flags require explicit cache invalidation. Paths
// count relative to the configuration's directory, never absolute, so every
// checkout with the same content has the same fingerprint.
func Fingerprint(configPath string, config []byte) (string, error) {
	// Parse local paths with the same YAML representation used by the builder.
	var inputs struct {
		Plugins []struct {
			Path string `yaml:"path"`
		} `yaml:"plugins"`
	}
	if err := yaml.Unmarshal(config, &inputs); err != nil {
		return "", errors.Wrap(err, "parse custom golangci-lint configuration")
	}
	hash := sha256.New()
	write := func(data []byte) {
		hash.Write(strconv.AppendInt(nil, int64(len(data)), 10))
		hash.Write([]byte{0})
		hash.Write(data)
	}
	write([]byte(runtime.Version()))
	write(config)

	// Plugin paths are hashed relative to the configuration's directory, so the
	// same content in two checkouts gives one fingerprint and they can share a
	// build.
	configDir, err := filepath.EvalSymlinks(filepath.Dir(configPath))
	if err != nil {
		return "", errors.Wrap(err, "resolve golangci-lint configuration directory")
	}

	// Walk plugin files in lexical order, excluding build caches and dependencies.
	for _, plugin := range inputs.Plugins {
		if plugin.Path == "" {
			continue
		}
		root := plugin.Path
		if !filepath.IsAbs(root) {
			root = filepath.Join(filepath.Dir(configPath), root)
		}
		root, err := filepath.EvalSymlinks(root)
		if err != nil {
			return "", errors.Wrap(err, "resolve local golangci-lint plugin")
		}
		err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			// Skip build caches, dependencies and symlinks to directories.
			if err != nil {
				return err
			}
			if entry.IsDir() {
				switch entry.Name() {
				case ".git", ".tmp", ".tools", "node_modules", "vendor":
					return filepath.SkipDir
				}
				return nil
			}
			if entry.Type()&fs.ModeSymlink != 0 {
				info, err := os.Stat(path)
				if err != nil {
					return err
				}
				if info.IsDir() {
					return nil
				}
			}

			// Hash each file by its path relative to the configuration and its bytes.
			data, err := os.ReadFile(path) //nolint:gosec // Local plugin sources are trusted build inputs; fingerprinting only reads them.
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(configDir, path)
			if err != nil {
				return err
			}
			write([]byte(filepath.ToSlash(rel)))
			write(data)
			return nil
		})
		if err != nil {
			return "", errors.Wrap(err, "fingerprint local golangci-lint plugin")
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
