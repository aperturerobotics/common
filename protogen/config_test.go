package protogen

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigGetGoModuleFromSubdir(t *testing.T) {
	t.Helper()

	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "net")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}

	goMod := []byte("module github.com/s4wave/spacewave\n\ngo 1.25.0\n")
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), goMod, 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	cfg := NewConfig()
	cfg.ProjectDir = projectDir

	moduleDir, err := cfg.GetModuleDir()
	if err != nil {
		t.Fatalf("get module dir: %v", err)
	}
	if moduleDir != tmpDir {
		t.Fatalf("expected module dir %q, got %q", tmpDir, moduleDir)
	}

	modulePath, err := cfg.GetGoModule()
	if err != nil {
		t.Fatalf("get go module: %v", err)
	}
	if modulePath != "github.com/s4wave/spacewave/net" {
		t.Fatalf("expected module path %q, got %q", "github.com/s4wave/spacewave/net", modulePath)
	}

	hasGoMod, err := cfg.HasGoMod()
	if err != nil {
		t.Fatalf("has go mod: %v", err)
	}
	if !hasGoMod {
		t.Fatal("expected has go mod to be true")
	}
}
