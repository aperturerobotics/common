package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/aperturerobotics/common/protogen"
)

func TestGenerateGoOnly(t *testing.T) {
	t.Helper()

	projectDir := t.TempDir()
	rootDir := repoRoot(t)

	goMod := []byte("module example.com/scratch\n\ngo 1.25.0\n\nrequire github.com/aperturerobotics/common v0.0.0\n\nreplace github.com/aperturerobotics/common => " + filepath.ToSlash(rootDir) + "\n")
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), goMod, 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	protoFile := []byte(`syntax = "proto3";
package scratch;

option go_package = "example.com/scratch";

message Scratch {
  string value = 1;
}
`)
	if err := os.WriteFile(filepath.Join(projectDir, "scratch.proto"), protoFile, 0o644); err != nil {
		t.Fatalf("write scratch.proto: %v", err)
	}
	runTestCommand(t, projectDir, "git", "init")
	runTestCommand(t, projectDir, "git", "add", "scratch.proto")

	cfg := protogen.NewConfig()
	cfg.ProjectDir = projectDir
	cfg.Force = true
	cfg.Languages = []string{"go"}

	if err := ensureDeps(cfg.ProjectDir, cfg.ToolsDir, false); err != nil {
		t.Fatalf("ensure deps: %v", err)
	}

	gen, err := protogen.NewGenerator(cfg)
	if err != nil {
		t.Fatalf("new generator: %v", err)
	}
	if err := gen.Generate(t.Context()); err != nil {
		t.Fatalf("generate: %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(projectDir, "scratch*"))
	if err != nil {
		t.Fatalf("glob generated files: %v", err)
	}

	expected := map[string]struct{}{
		"scratch.pb.go":      {},
		"scratch.proto":      {},
		"scratch_srpc.pb.go": {},
	}
	for _, match := range matches {
		base := filepath.Base(match)
		if _, ok := expected[base]; !ok {
			t.Fatalf("unexpected Go-only output %s in %v", base, matches)
		}
		delete(expected, base)
	}
	if _, ok := expected["scratch.pb.go"]; ok {
		t.Fatalf("missing generated scratch.pb.go in %v", matches)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("get caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func runTestCommand(t *testing.T, dir, name string, args ...string) {
	t.Helper()

	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, output)
	}
}
