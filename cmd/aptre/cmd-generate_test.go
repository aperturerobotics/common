package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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

func TestGenerateGoOnlyNoRPC(t *testing.T) {
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
	cfg.RPCLibraries = []string{"none"}

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
		"scratch.pb.go": {},
		"scratch.proto": {},
	}
	for _, match := range matches {
		base := filepath.Base(match)
		if _, ok := expected[base]; !ok {
			t.Fatalf("unexpected no-RPC output %s in %v", base, matches)
		}
		delete(expected, base)
	}
	for missing := range expected {
		t.Fatalf("missing generated %s in %v", missing, matches)
	}
}

func TestGenerateCSharpAndPython(t *testing.T) {
	t.Helper()

	projectDir := t.TempDir()
	goMod := []byte("module example.com/play-scratch\n\ngo 1.25.0\n")
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), goMod, 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	protoFile := []byte(`syntax = "proto3";
package scratch;

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
	cfg.Languages = []string{"csharp", "python"}

	gen, err := protogen.NewGenerator(cfg)
	if err != nil {
		t.Fatalf("new generator: %v", err)
	}
	if err := gen.Generate(t.Context()); err != nil {
		t.Fatalf("generate: %v", err)
	}

	csharpPath := filepath.Join(projectDir, "Scratch.cs")
	pythonPath := filepath.Join(projectDir, "scratch_pb2.py")
	csharp, err := os.ReadFile(csharpPath)
	if err != nil {
		t.Fatalf("read C# output: %v", err)
	}
	python, err := os.ReadFile(pythonPath)
	if err != nil {
		var files []string
		_ = filepath.WalkDir(projectDir, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr == nil && !entry.IsDir() {
				files = append(files, path)
			}
			return nil
		})
		t.Fatalf("read Python output: %v; files: %v", err, files)
	}
	if !bytes.Contains(csharp, []byte("class Scratch")) {
		t.Fatal("C# output does not contain Scratch")
	}
	if !bytes.Contains(python, []byte("Scratch")) {
		t.Fatal("Python output does not contain Scratch")
	}

	var stdout bytes.Buffer
	gen, err = protogen.NewGenerator(cfg)
	if err != nil {
		t.Fatalf("new cached generator: %v", err)
	}
	gen.Verbose = true
	gen.Stdout = &stdout
	if err := gen.Generate(t.Context()); err != nil {
		t.Fatalf("cached generate: %v", err)
	}
	if !strings.Contains(stdout.String(), "Skipping . (up to date)") {
		t.Fatalf("expected second-run cache reuse, got %q", stdout.String())
	}
	if got, err := os.ReadFile(csharpPath); err != nil || !bytes.Equal(got, csharp) {
		t.Fatalf("C# second-run output changed: %v", err)
	}
	if got, err := os.ReadFile(pythonPath); err != nil || !bytes.Equal(got, python) {
		t.Fatalf("Python second-run output changed: %v", err)
	}

	cfg.Languages = []string{"csharp"}
	gen, err = protogen.NewGenerator(cfg)
	if err != nil {
		t.Fatalf("new C# generator: %v", err)
	}
	if err := gen.Generate(t.Context()); err != nil {
		t.Fatalf("C# generate: %v", err)
	}
	if _, err := os.Stat(pythonPath); !os.IsNotExist(err) {
		t.Fatalf("expected stale Python output removal, got %v", err)
	}
	if got, err := os.ReadFile(csharpPath); err != nil || !bytes.Equal(got, csharp) {
		t.Fatalf("C# output changed after language invalidation: %v", err)
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
