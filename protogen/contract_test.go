package protogen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverProtoFilesReportsTargetWithoutSources(t *testing.T) {
	dir := t.TempDir()
	runContractCommand(t, dir, "git", "init")
	if err := os.WriteFile(filepath.Join(dir, "present.proto"), []byte("syntax = \"proto3\";\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runContractCommand(t, dir, "git", "add", "present.proto")

	_, unmatched, err := discoverProtoFiles(dir, []string{"missing/*.proto"}, nil)
	if err != nil {
		t.Fatalf("discoverProtoFiles(): %v", err)
	}
	if len(unmatched) != 1 || unmatched[0] != "missing/*.proto" {
		t.Fatalf("unmatched targets = %v, want missing target", unmatched)
	}
}

func TestDiscoverProtoFilesAdmitsExplicitUntrackedSource(t *testing.T) {
	dir := t.TempDir()
	runContractCommand(t, dir, "git", "init")
	if err := os.WriteFile(filepath.Join(dir, "untracked.proto"), []byte("syntax = \"proto3\";\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, unmatched, err := discoverProtoFiles(dir, []string{"untracked.proto"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(unmatched) != 0 || len(files) != 1 || files[0] != "untracked.proto" {
		t.Fatalf("files=%v unmatched=%v, want explicit untracked source", files, unmatched)
	}
}

func TestDiscoverProtoFilesRejectsEscapingTarget(t *testing.T) {
	dir := t.TempDir()
	_, _, err := discoverProtoFiles(dir, []string{"../outside.proto"}, nil)
	if err == nil || !strings.Contains(err.Error(), "escapes project directory") {
		t.Fatalf("discoverProtoFiles() error = %v, want escape diagnostic", err)
	}
}

func TestDiscoverProtoFilesRejectsSymlinkParentEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	runContractCommand(t, root, "git", "init")
	if err := os.WriteFile(filepath.Join(outside, "outside.proto"), []byte("syntax = \"proto3\";\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}

	_, _, err := discoverProtoFiles(root, []string{"linked/outside.proto"}, nil)
	if err == nil || !strings.Contains(err.Error(), "escapes project directory") {
		t.Fatalf("discoverProtoFiles() error = %v, want physical escape diagnostic", err)
	}
}

func TestGeneratorKeepsDefaultEmptyProjectBehavior(t *testing.T) {
	dir := t.TempDir()
	runContractCommand(t, dir, "git", "init")
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/contracts\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gen := &Generator{
		Config:     NewConfig(),
		ProjectDir: dir,
		ModulePath: "example.com/contracts",
		VendorDir:  filepath.Join(dir, "vendor"),
	}
	if err := gen.Generate(t.Context()); err != nil {
		t.Fatalf("Generate() default empty project: %v", err)
	}
}

func TestGeneratorRejectsExplicitUnmatchedTarget(t *testing.T) {
	dir := t.TempDir()
	runContractCommand(t, dir, "git", "init")
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/contracts\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	gen := &Generator{
		Config: &Config{
			Targets:         []string{"missing/*.proto"},
			TargetsExplicit: true,
		},
		ProjectDir: dir,
		ModulePath: "example.com/contracts",
		VendorDir:  filepath.Join(dir, "vendor"),
	}
	if err := gen.Generate(t.Context()); err == nil || !strings.Contains(err.Error(), "matched no proto sources") {
		t.Fatalf("Generate() error = %v, want unmatched-target diagnostic", err)
	}
}

func runContractCommand(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, output)
	}
}
