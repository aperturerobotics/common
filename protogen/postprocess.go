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

// goImportRemaps maps standard protobuf Go imports to protobuf-go-lite equivalents.
var goImportRemaps = map[string]string{
	"google.golang.org/protobuf/types/known/emptypb":     "github.com/aperturerobotics/protobuf-go-lite/types/known/emptypb",
	"google.golang.org/protobuf/types/known/anypb":       "github.com/aperturerobotics/protobuf-go-lite/types/known/anypb",
	"google.golang.org/protobuf/types/known/durationpb":  "github.com/aperturerobotics/protobuf-go-lite/types/known/durationpb",
	"google.golang.org/protobuf/types/known/timestamppb": "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb",
	"google.golang.org/protobuf/types/known/wrapperspb":  "github.com/aperturerobotics/protobuf-go-lite/types/known/wrapperspb",
	"google.golang.org/protobuf/types/known/structpb":    "github.com/aperturerobotics/protobuf-go-lite/types/known/structpb",
	"google.golang.org/protobuf/types/known/fieldmaskpb": "github.com/aperturerobotics/protobuf-go-lite/types/known/fieldmaskpb",
}

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
	cppFiles, err := filepath.Glob(filepath.Join(searchDir, baseName+"*.pb.cpp"))
	if err != nil {
		return err
	}
	hFiles, err := filepath.Glob(filepath.Join(searchDir, baseName+"*.pb.h"))
	if err != nil {
		return err
	}
	hppFiles, err := filepath.Glob(filepath.Join(searchDir, baseName+"*.pb.hpp"))
	if err != nil {
		return err
	}

	allCppFiles := append(ccFiles, cppFiles...)
	allCppFiles = append(allCppFiles, hFiles...)
	allCppFiles = append(allCppFiles, hppFiles...)
	for _, f := range allCppFiles {
		if err := p.ProcessCppFile(f); err != nil {
			return err
		}
	}

	// Process Go files
	goFiles, err := filepath.Glob(filepath.Join(searchDir, baseName+"*.pb.go"))
	if err != nil {
		return err
	}

	for _, f := range goFiles {
		if err := p.ProcessGoFile(f); err != nil {
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

	// Process Rust files (move from package-based path to proto-based path)
	if err := p.ProcessRustFiles(protoFile); err != nil {
		return err
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

	// Check and add build tag if no Go build tag is present
	hasBuildTag := len(lines) > 0 && strings.HasPrefix(lines[0], "//go:build ")
	if !hasBuildTag {
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

// ProcessGoFile processes a Go file.
// Rewrites standard protobuf imports to protobuf-go-lite equivalents.
func (p *PostProcessor) ProcessGoFile(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	content := string(data)
	modified := false

	// Replace each import path
	for oldImport, newImport := range goImportRemaps {
		if strings.Contains(content, oldImport) {
			content = strings.ReplaceAll(content, oldImport, newImport)
			modified = true
		}
	}

	if modified {
		return os.WriteFile(filePath, []byte(content), 0o644)
	}

	return nil
}

// ProcessTsFile processes a TypeScript file.
// Rewrites relative import paths to @go/ format for vendor dependencies.
//
// The generated TypeScript files contain relative imports based on the proto file paths.
// For example, a file generated from "github.com/aperturerobotics/bifrost/daemon/api/api.proto"
// might import from "../../../controllerbus/bus/api/api.pb.js" which needs to be rewritten
// to "@go/github.com/aperturerobotics/controllerbus/bus/api/api.pb.js".
func (p *PostProcessor) ProcessTsFile(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	fileDir := filepath.Dir(filePath)
	content := string(data)

	// Extract the proto source path from the generated file header.
	// The header looks like: // @generated from file github.com/aperturerobotics/bifrost/daemon/api/api.proto
	sourceProtoPath := extractProtoSourcePath(content)
	if sourceProtoPath == "" {
		// No source path found, skip processing
		return nil
	}

	// Get the directory of the proto file (e.g., "github.com/aperturerobotics/bifrost/daemon/api")
	protoDir := filepath.Dir(sourceProtoPath)

	// Pattern to match any relative imports starting with ../
	importPattern := regexp.MustCompile(`from\s+"(\.\.\/[^"]+)"`)

	modified := false
	var result bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(data))

	for scanner.Scan() {
		line := scanner.Text()
		matches := importPattern.FindStringSubmatch(line)

		if len(matches) > 1 {
			importPath := matches[1]

			// Resolve the import path relative to the proto directory to get the full Go import path.
			// For example:
			//   protoDir = "github.com/aperturerobotics/bifrost/daemon/api"
			//   importPath = "../../../controllerbus/bus/api/api.pb.js"
			//   result = "github.com/aperturerobotics/controllerbus/bus/api/api.pb.js"
			resolvedPath := resolveRelativeImport(protoDir, importPath)

			// Check if this resolves to outside our module (i.e., it's a vendor dependency)
			if !strings.HasPrefix(resolvedPath, p.ModulePath) {
				// This is an external import - verify it exists in vendor and rewrite to @go/ format
				vendorFilePath := filepath.Join(p.VendorDir, resolvedPath)
				// Check for .ts file (the .js extension in import maps to .ts source)
				tsPath := strings.TrimSuffix(vendorFilePath, ".js") + ".ts"

				if fileExists(tsPath) || fileExists(vendorFilePath) {
					goImportPath := "@go/" + resolvedPath
					newLine := strings.Replace(line, importPath, goImportPath, 1)
					if newLine != line {
						line = newLine
						modified = true
					}
				}
			} else {
				// This is an internal import within the same module.
				// Check if it resolves to the vendor directory (self-referencing via full path).
				absImportPath := filepath.Clean(filepath.Join(fileDir, importPath))
				relToProject, err := filepath.Rel(p.ProjectDir, absImportPath)
				if err == nil && strings.HasPrefix(relToProject, "vendor/") {
					vendorPath := strings.TrimPrefix(relToProject, "vendor/")
					goImportPath := "@go/" + filepath.ToSlash(vendorPath)
					newLine := strings.Replace(line, importPath, goImportPath, 1)
					if newLine != line {
						line = newLine
						modified = true
					}
				}
				// Otherwise, leave internal relative imports as-is
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

// extractProtoSourcePath extracts the proto source path from a generated file's header.
// It looks for a line like: // @generated from file github.com/aperturerobotics/bifrost/daemon/api/api.proto
func extractProtoSourcePath(content string) string {
	// Match the @generated from file comment
	pattern := regexp.MustCompile(`@generated from file ([^\s(]+\.proto)`)
	matches := pattern.FindStringSubmatch(content)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// resolveRelativeImport resolves a relative import path against a base directory path.
// For example:
//
//	baseDir = "github.com/aperturerobotics/bifrost/daemon/api"
//	importPath = "../../../controllerbus/bus/api/api.pb.js"
//	result = "github.com/aperturerobotics/controllerbus/bus/api/api.pb.js"
func resolveRelativeImport(baseDir, importPath string) string {
	// Split the base directory into parts
	parts := strings.Split(baseDir, "/")

	// Process each ../ in the import path
	remaining := importPath
	for strings.HasPrefix(remaining, "../") {
		remaining = strings.TrimPrefix(remaining, "../")
		if len(parts) > 0 {
			parts = parts[:len(parts)-1]
		}
	}

	// Combine the remaining base path with the import path
	if len(parts) > 0 {
		return strings.Join(parts, "/") + "/" + remaining
	}
	return remaining
}

// fileExists checks if a file exists and is not a directory.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
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
		if strings.HasSuffix(path, ".pb.cc") || strings.HasSuffix(path, ".pb.cpp") ||
			strings.HasSuffix(path, ".pb.h") || strings.HasSuffix(path, ".pb.hpp") {
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

// ProcessRustFiles moves and processes generated Rust .pb.rs files.
// Prost generates files at vendor/{package_path}/{basename}.pb.rs based on the
// protobuf package hierarchy, but we need them at vendor/{module_path}/{proto_dir}/
// to match other generated files.
func (p *PostProcessor) ProcessRustFiles(protoFile string) error {
	protoDir := filepath.Dir(protoFile)
	baseName := strings.TrimSuffix(filepath.Base(protoFile), ".proto")

	// Get the proto package from the proto file
	protoPackage, err := extractProtoPackage(filepath.Join(p.ProjectDir, protoFile))
	if err != nil {
		return err
	}
	if protoPackage == "" {
		return nil // No package, skip
	}

	// Prost generates files at vendor/{package_path}/{basename}.pb.rs
	// where package_path is the package name converted to directory structure
	packagePath := strings.ReplaceAll(protoPackage, ".", "/")
	srcFile := filepath.Join(p.VendorDir, packagePath, baseName+".pb.rs")

	// Target location is vendor/{module_path}/{proto_dir}/{basename}.pb.rs
	dstDir := filepath.Join(p.VendorDir, p.ModulePath, protoDir)
	dstFile := filepath.Join(dstDir, baseName+".pb.rs")

	// Check if source file exists
	if !fileExists(srcFile) {
		return nil // No rust file generated, skip
	}

	// Create destination directory
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}

	// Read source file
	data, err := os.ReadFile(srcFile)
	if err != nil {
		return err
	}

	// Write to destination
	if err := os.WriteFile(dstFile, data, 0o644); err != nil {
		return err
	}

	// Remove source file
	if err := os.Remove(srcFile); err != nil {
		return err
	}

	// Try to remove empty parent directories
	p.cleanEmptyDirs(filepath.Dir(srcFile))

	return nil
}

// cleanEmptyDirs removes empty directories up to the vendor dir.
func (p *PostProcessor) cleanEmptyDirs(dir string) {
	for dir != p.VendorDir && strings.HasPrefix(dir, p.VendorDir) {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			break
		}
		os.Remove(dir)
		dir = filepath.Dir(dir)
	}
}

// extractProtoPackage extracts the package name from a proto file.
func extractProtoPackage(protoPath string) (string, error) {
	data, err := os.ReadFile(protoPath)
	if err != nil {
		return "", err
	}

	// Match: package example.other;
	pattern := regexp.MustCompile(`(?m)^package\s+([a-zA-Z0-9_.]+)\s*;`)
	matches := pattern.FindSubmatch(data)
	if len(matches) > 1 {
		return string(matches[1]), nil
	}
	return "", nil
}
