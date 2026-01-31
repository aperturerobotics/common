package protogen

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
)

// DiscoverProtoFiles finds proto files matching the given patterns.
// Uses git ls-files to find tracked proto files.
// excludePatterns allows excluding files that match certain patterns.
func DiscoverProtoFiles(projectDir string, patterns, excludePatterns []string) ([]string, error) {
	var allFiles []string
	seen := make(map[string]struct{})

	for _, pattern := range patterns {
		files, err := discoverPattern(projectDir, pattern)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			if _, ok := seen[f]; !ok {
				// Check if file matches any exclude pattern
				if matchesAnyPattern(f, excludePatterns) {
					continue
				}
				seen[f] = struct{}{}
				allFiles = append(allFiles, f)
			}
		}
	}

	return allFiles, nil
}

// matchesAnyPattern checks if a file path matches any of the given glob patterns.
func matchesAnyPattern(filePath string, patterns []string) bool {
	for _, pattern := range patterns {
		// Try matching against the full path
		matched, err := filepath.Match(pattern, filePath)
		if err == nil && matched {
			return true
		}
		// Also try matching against just the filename
		matched, err = filepath.Match(pattern, filepath.Base(filePath))
		if err == nil && matched {
			return true
		}
	}
	return false
}

// discoverPattern finds proto files matching a single pattern using git ls-files.
func discoverPattern(projectDir, pattern string) ([]string, error) {
	cmd := exec.Command("git", "ls-files", pattern)
	cmd.Dir = projectDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// If git ls-files fails, fall back to filepath.Glob
		return filepath.Glob(filepath.Join(projectDir, pattern))
	}

	var files []string
	scanner := bufio.NewScanner(&stdout)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && strings.HasSuffix(line, ".proto") {
			files = append(files, line)
		}
	}

	return files, scanner.Err()
}

// GetGoModule reads the module path from go.mod in the given directory.
func GetGoModule(projectDir string) (string, error) {
	goModPath := filepath.Join(projectDir, "go.mod")
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return "", err
	}

	modFile, err := modfile.ParseLax(goModPath, data, nil)
	if err != nil {
		return "", err
	}

	return modFile.Module.Mod.Path, nil
}

// GetGeneratedFiles returns the expected generated file paths for a proto file.
func GetGeneratedFiles(protoFile, projectDir, modulePath string, hasGo, hasTS bool) []string {
	protoDir := filepath.Dir(protoFile)
	baseName := strings.TrimSuffix(filepath.Base(protoFile), ".proto")

	var files []string

	// C++ files are always generated
	files = append(files,
		filepath.Join("vendor", modulePath, protoDir, baseName+".pb.cc"),
		filepath.Join("vendor", modulePath, protoDir, baseName+".pb.h"),
	)

	if hasGo {
		files = append(files,
			filepath.Join("vendor", modulePath, protoDir, baseName+".pb.go"),
			filepath.Join("vendor", modulePath, protoDir, baseName+"_srpc.pb.go"),
		)
	}

	if hasTS {
		files = append(files,
			filepath.Join("vendor", modulePath, protoDir, baseName+".pb.ts"),
			filepath.Join("vendor", modulePath, protoDir, baseName+"_srpc.pb.ts"),
		)
	}

	return files
}

// FindGeneratedFilesForProto finds actual generated files for a proto file using glob.
func FindGeneratedFilesForProto(protoFile, projectDir, modulePath string) ([]string, error) {
	protoDir := filepath.Dir(protoFile)
	baseName := strings.TrimSuffix(filepath.Base(protoFile), ".proto")

	searchDir := filepath.Join(projectDir, "vendor", modulePath, protoDir)
	pattern := filepath.Join(searchDir, baseName+"*.pb.*")

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	// Also check for files in the proto directory itself (not vendor)
	localPattern := filepath.Join(projectDir, protoDir, baseName+"*.pb.*")
	localMatches, err := filepath.Glob(localPattern)
	if err != nil {
		return nil, err
	}
	matches = append(matches, localMatches...)

	// Convert to relative paths
	var relPaths []string
	for _, m := range matches {
		rel, err := filepath.Rel(projectDir, m)
		if err != nil {
			rel = m
		}
		relPaths = append(relPaths, rel)
	}

	return relPaths, nil
}
