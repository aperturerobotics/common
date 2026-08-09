package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunLintChecksSourceBeforeBuildingTools(t *testing.T) {
	projectDir := t.TempDir()
	rootDir := repoRoot(t)
	goMod := "module example.com/lint-order\n\ngo 1.25.0\n\nrequire github.com/aperturerobotics/common v0.0.0\n\nreplace github.com/aperturerobotics/common => " + filepath.ToSlash(rootDir) + "\n"
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "invalid.go"), []byte("package fixture\n\nfunc invalid() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runLintGit(t, projectDir, "init", "--quiet")
	runLintGit(t, projectDir, "add", "go.mod", "invalid.go")

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "run", ".", "lint", "--project-dir", projectDir)
	cmd.Dir = filepath.Join(rootDir, "cmd", "aptre")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("aptre lint unexpectedly accepted invalid source:\n%s", output)
	}
	if !strings.Contains(string(output), "invalid.go") {
		t.Fatalf("aptre lint did not return the source diagnostic first: %v\n%s", err, output)
	}
	if _, statErr := os.Lstat(filepath.Join(projectDir, ".tools")); !os.IsNotExist(statErr) {
		t.Fatalf("aptre lint constructed tools before source validation: %v", statErr)
	}
}

func runLintGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}
