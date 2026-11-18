package protogen

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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
	// Ask Git for tracked files matching this pattern.
	cmd := exec.Command("git", "ls-files", pattern)
	cmd.Dir = projectDir

	// Capture the tracked paths and any command diagnostics.
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Use a filesystem glob when Git cannot list the project.
	if err := cmd.Run(); err != nil {
		// If git ls-files fails, fall back to filepath.Glob
		return filepath.Glob(filepath.Join(projectDir, pattern))
	}

	// Keep nonempty proto paths from the tracked-file output.
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
	// Read the project's module declaration.
	goModPath := filepath.Join(projectDir, "go.mod")
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return "", err
	}

	// Parse the module path without requiring a complete module file.
	modFile, err := modfile.ParseLax(goModPath, data, nil)
	if err != nil {
		return "", err
	}

	return modFile.Module.Mod.Path, nil
}

// FindGeneratedFilesForProto finds actual enabled outputs for a proto file.
func FindGeneratedFilesForProto(protoFile, projectDir string, langs Languages, rpcs RPCLibraries) ([]string, error) {
	// Derive the directory and output stem shared by the generators.
	protoDir := filepath.Dir(protoFile)
	baseName := strings.TrimSuffix(filepath.Base(protoFile), ".proto")

	// Match protoc's C# filename casing and word boundaries.
	var csharpBase strings.Builder
	capNext := true
	for i := range len(baseName) {
		c := baseName[i]
		switch {
		case c >= 'a' && c <= 'z':
			if capNext {
				c -= 'a' - 'A'
			}
			csharpBase.WriteByte(c)
			capNext = false
		case c >= 'A' && c <= 'Z':
			csharpBase.WriteByte(c)
			capNext = false
		case c >= '0' && c <= '9':
			csharpBase.WriteByte(c)
			capNext = true
		default:
			capNext = true
		}
	}
	csharpFile := csharpBase.String()
	if csharpFile != "" && csharpFile[0] >= '0' && csharpFile[0] <= '9' &&
		strings.HasPrefix(baseName, "_") {
		csharpFile = "_" + csharpFile
	}

	// Select only patterns produced by enabled generators.
	var relativePatterns []string
	if langs.Has(LanguageCpp) {
		relativePatterns = append(relativePatterns, baseName+".pb.cc", baseName+".pb.h")
	}
	if langs.Has(LanguageGo) {
		relativePatterns = append(relativePatterns, baseName+"*.pb.go")
	}
	if langs.Has(LanguageTypeScript) {
		relativePatterns = append(relativePatterns, baseName+"*.pb.ts")
	}
	if langs.Has(LanguageRust) {
		relativePatterns = append(relativePatterns, baseName+"*.pb.rs")
	}
	if langs.Has(LanguagePython) {
		relativePatterns = append(relativePatterns, baseName+"_pb2.py", baseName+"_pb2.pyi")
		if rpcs.Has(RPCLibraryStarpcPython) {
			relativePatterns = append(relativePatterns, baseName+"_srpc.py", baseName+"_srpc.pyi")
		}
	}

	// Search the schema's directory, and the project root for C#.
	searches := []struct {
		dir      string
		patterns []string
	}{
		{dir: filepath.Join(projectDir, protoDir), patterns: relativePatterns},
	}
	if langs.Has(LanguageCSharp) {
		searches = append(searches, struct {
			dir      string
			patterns []string
		}{
			dir:      projectDir,
			patterns: []string{csharpFile + ".cs"},
		})
	}

	// Return stable project-relative output paths.
	var relPaths []string
	for _, search := range searches {
		for _, pattern := range search.patterns {
			matches, err := filepath.Glob(filepath.Join(search.dir, pattern))
			if err != nil {
				return nil, err
			}
			for _, match := range matches {
				rel, err := filepath.Rel(projectDir, match)
				if err != nil {
					return nil, err
				}
				relPaths = append(relPaths, rel)
			}
		}
	}
	slices.Sort(relPaths)
	return slices.Compact(relPaths), nil
}
