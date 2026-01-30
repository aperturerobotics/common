package protogen

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	// CppBuildTag is the build tag prepended to C++ files.
	CppBuildTag = "//go:build deps_only && cgo"
)

// PostProcessor handles post-processing of generated files.
type PostProcessor struct {
	// ProjectDir is the project directory.
	ProjectDir string
	// ModulePath is the Go module path.
	ModulePath string
	// VendorDir is the vendor directory path.
	VendorDir string
	// Verbose enables verbose output.
	Verbose bool
}

// NewPostProcessor creates a new PostProcessor.
func NewPostProcessor(projectDir, modulePath string, verbose bool) *PostProcessor {
	return &PostProcessor{
		ProjectDir: projectDir,
		ModulePath: modulePath,
		VendorDir:  filepath.Join(projectDir, "vendor"),
		Verbose:    verbose,
	}
}

// ProcessGeneratedFiles processes all generated files for a proto file.
func (p *PostProcessor) ProcessGeneratedFiles(protoFile string) error {
	protoDir := filepath.Dir(protoFile)
	baseName := strings.TrimSuffix(filepath.Base(protoFile), ".proto")

	// Process C++ files
	searchDir := filepath.Join(p.ProjectDir, protoDir)
	ccFiles, err := filepath.Glob(filepath.Join(searchDir, baseName+"*.pb.cc"))
	if err != nil {
		return err
	}
	hFiles, err := filepath.Glob(filepath.Join(searchDir, baseName+"*.pb.h"))
	if err != nil {
		return err
	}

	for _, f := range append(ccFiles, hFiles...) {
		if err := p.ProcessCppFile(f); err != nil {
			return err
		}
	}

	// Process TypeScript files
	tsFiles, err := filepath.Glob(filepath.Join(searchDir, baseName+"*.pb.ts"))
	if err != nil {
		return err
	}

	for _, f := range tsFiles {
		if err := p.ProcessTsFile(f); err != nil {
			return err
		}
	}

	return nil
}

// ProcessCppFile processes a C++ file.
// - Prepends the Go build tag if not present.
// - Rewrites include paths from absolute to relative.
func (p *PostProcessor) ProcessCppFile(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	modified := false
	lines := strings.Split(string(data), "\n")

	// Check and add build tag
	if len(lines) == 0 || lines[0] != CppBuildTag {
		lines = append([]string{CppBuildTag, ""}, lines...)
		modified = true
	}

	// Get the directory of this file relative to the project dir
	fileRelDir, err := filepath.Rel(p.ProjectDir, filepath.Dir(filePath))
	if err != nil {
		fileRelDir = filepath.Dir(filePath)
	}

	// Rewrite include paths
	// Match includes like: #include "github.com/aperturerobotics/common/example/file.pb.h"
	includePattern := regexp.MustCompile(`#include "` + regexp.QuoteMeta(p.ModulePath) + `/([^"]+\.pb\.h)"`)

	for i, line := range lines {
		matches := includePattern.FindStringSubmatch(line)
		if len(matches) > 1 {
			// Found an include with absolute module path
			// includedFile is the path relative to module root, e.g., "example/file.pb.h"
			includedFile := matches[1]
			includedDir := filepath.Dir(includedFile)

			// Calculate relative path from this file's directory to the included file
			var relPath string
			if fileRelDir == includedDir {
				// Same directory - just use the filename
				relPath = filepath.Base(includedFile)
			} else {
				// Different directory - calculate relative path
				relPath, err = filepath.Rel(fileRelDir, includedFile)
				if err != nil {
					continue
				}
			}

			newLine := `#include "` + relPath + `"`
			if lines[i] != newLine {
				lines[i] = newLine
				modified = true
			}
		}
	}

	if modified {
		return os.WriteFile(filePath, []byte(strings.Join(lines, "\n")), 0o644)
	}

	return nil
}

// ProcessTsFile processes a TypeScript file.
// Rewrites relative import paths to @go/ format.
func (p *PostProcessor) ProcessTsFile(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	fileDir := filepath.Dir(filePath)

	// Calculate the depth of the file relative to the project root
	relPath, err := filepath.Rel(p.ProjectDir, fileDir)
	if err != nil {
		return err
	}

	// Count directory depth
	depth := strings.Count(relPath, string(filepath.Separator)) + 1

	// Build the prefix pattern (e.g., "../../../" for depth 3)
	var prefixParts []string
	for i := 0; i < depth; i++ {
		prefixParts = append(prefixParts, "..")
	}
	prefix := strings.Join(prefixParts, "/") + "/"

	// Pattern to match imports like: from "../../../path/to/file"
	importPattern := regexp.MustCompile(`from\s+"(` + regexp.QuoteMeta(prefix) + `[^"]+)"`)

	modified := false
	var result bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(data))

	for scanner.Scan() {
		line := scanner.Text()
		matches := importPattern.FindStringSubmatch(line)

		if len(matches) > 1 {
			importPath := matches[1]

			// Resolve the import path to get the actual path
			absImportPath := filepath.Join(fileDir, importPath)
			relToVendor, err := filepath.Rel(p.VendorDir, absImportPath)

			if err == nil && !strings.HasPrefix(relToVendor, "..") {
				// Convert to @go/ format
				goImportPath := "@go/" + filepath.ToSlash(relToVendor)
				newLine := strings.Replace(line, importPath, goImportPath, 1)
				if newLine != line {
					line = newLine
					modified = true
				}
			}
		}

		result.WriteString(line)
		result.WriteString("\n")
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	if modified {
		// Remove trailing newline added by loop
		output := result.Bytes()
		if len(output) > 0 && output[len(output)-1] == '\n' {
			output = output[:len(output)-1]
		}
		return os.WriteFile(filePath, output, 0o644)
	}

	return nil
}

// ProcessAllCppFiles finds and processes all C++ files in a directory.
func (p *PostProcessor) ProcessAllCppFiles(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".pb.cc") || strings.HasSuffix(path, ".pb.h") {
			return p.ProcessCppFile(path)
		}
		return nil
	})
}

// ProcessAllTsFiles finds and processes all TypeScript files in a directory.
func (p *PostProcessor) ProcessAllTsFiles(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".pb.ts") {
			return p.ProcessTsFile(path)
		}
		return nil
	})
}
