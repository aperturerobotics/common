package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// CommonFiles contains the set of embedded common files.
//
//go:embed tsconfig.json Makefile .eslintrc.js .eslintignore .gitignore
var CommonFiles embed.FS

// ExtractCommonFiles copies the contents of CommonFiles to the given output path.
func ExtractCommonFiles(outputPath string) error {
	return extractFiles(CommonFiles, outputPath, nil)
}

// ToolsFiles contains the set of tools deps files.
//
// We copy some files to use for the tools so that they are not interpeted as a separate go module.
//
//go:generate bash embed.bash
//go:embed deps.go.tools go.mod.tools go.sum.tools
var ToolsFiles embed.FS

// ExtractToolsFiles copies the contents of ToolsFiles to the given output path.
func ExtractToolsFiles(outputPath string) error {
	return extractFiles(ToolsFiles, outputPath, func(path string) string {
		return strings.TrimSuffix(path, ".tools")
	})
}

// extractFiles is a helper function that copies the contents of an embed.FS to the given output path.
// It takes an optional remapPath function to remap file paths for the output.
func extractFiles(fsys embed.FS, outputPath string, remapPath func(string) string) error {
	return fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		outputFilePath := filepath.Join(outputPath, path)
		if remapPath != nil {
			outputFilePath = filepath.Join(outputPath, remapPath(path))
		}

		outputDir := filepath.Dir(outputFilePath)

		err = os.MkdirAll(outputDir, os.ModePerm)
		if err != nil {
			return err
		}

		inputFile, err := fsys.Open(path)
		if err != nil {
			return err
		}
		defer inputFile.Close()

		outputFile, err := os.OpenFile(outputFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			return err
		}

		_, err = outputFile.ReadFrom(inputFile)
		if err != nil {
			_ = outputFile.Close()
			return err
		}

		return outputFile.Close()
	})
}

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: go run main.go <output_directory>")
		os.Exit(1)
	}

	outputPath := os.Args[1]

	err := ExtractCommonFiles(outputPath)
	if err != nil {
		fmt.Printf("Error extracting common files: %v\n", err)
		os.Exit(1)
	}

	err = ExtractToolsFiles(outputPath)
	if err != nil {
		fmt.Printf("Error extracting tools files: %v\n", err)
		os.Exit(1)
	}
}
