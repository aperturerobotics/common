package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCMakeBuild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping cmake test on windows")
	}

	// Check if cmake is available
	if _, err := exec.LookPath("cmake"); err != nil {
		t.Skip("cmake not found, skipping test")
	}

	// Get the directory containing this test file (project root)
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to get current file path")
	}
	projectDir := filepath.Dir(filename)

	// Create a temporary build directory
	buildDir, err := os.MkdirTemp("", "cmake-build-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(buildDir)

	// Run cmake configure
	cmake := exec.Command("cmake", projectDir)
	cmake.Dir = buildDir
	cmake.Stdout = os.Stdout
	cmake.Stderr = os.Stderr
	if err := cmake.Run(); err != nil {
		t.Fatalf("cmake configure failed: %v", err)
	}

	// Run cmake build
	build := exec.Command("cmake", "--build", ".")
	build.Dir = buildDir
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("cmake build failed: %v", err)
	}
}
