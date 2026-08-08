package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/aperturerobotics/common/protogen"
	"github.com/pkg/errors"
)

func TestPlanGenerateDependenciesByLanguageAndRPC(t *testing.T) {
	tests := []struct {
		name        string
		languages   []string
		rpcs        []string
		packageJSON bool
		wantTools   []string
		wantNode    bool
	}{
		{name: "python only", languages: []string{"python"}},
		{name: "csharp only", languages: []string{"csharp"}},
		{name: "go without rpc", languages: []string{"go"}, rpcs: []string{"none"}, wantTools: []string{"protoc-gen-go-lite", "gofumpt"}},
		{name: "typescript", languages: []string{"ts"}, packageJSON: true, wantNode: true},
		{name: "mixed", languages: []string{"go", "ts"}, packageJSON: true, wantTools: []string{"protoc-gen-go-lite", "protoc-gen-go-starpc", "gofumpt"}, wantNode: true},
		{name: "default", packageJSON: true, wantTools: []string{"protoc-gen-go-lite", "protoc-gen-go-starpc", "gofumpt", "protoc-gen-starpc-cpp", "protoc-gen-starpc-rust"}, wantNode: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectDir := t.TempDir()
			if tt.packageJSON {
				if err := os.WriteFile(filepath.Join(projectDir, "package.json"), []byte(`{"private":true}`), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			cfg := protogen.NewConfig()
			cfg.ProjectDir = projectDir
			cfg.Languages = tt.languages
			cfg.RPCLibraries = tt.rpcs
			got, err := planGenerateDependencies(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got.nativeTools, tt.wantTools) {
				t.Fatalf("native tools = %v, want %v", got.nativeTools, tt.wantTools)
			}
			if got.ensureNodeModules != tt.wantNode {
				t.Fatalf("ensure node_modules = %v, want %v", got.ensureNodeModules, tt.wantNode)
			}
		})
	}
}

func TestEnsureGenerateDepsPythonOnlyCreatesNoToolOrNodeDirectories(t *testing.T) {
	projectDir := t.TempDir()
	cfg := protogen.NewConfig()
	cfg.ProjectDir = projectDir
	cfg.ToolsDir = ".tools"
	cfg.Languages = []string{"python"}

	if err := ensureGenerateDeps(cfg, false); err != nil {
		t.Fatalf("ensure Python-only dependencies: %v", err)
	}
	for _, name := range []string{".tools", "node_modules"} {
		if _, err := os.Stat(filepath.Join(projectDir, name)); !os.IsNotExist(err) {
			t.Fatalf("Python-only setup created %s: %v", name, err)
		}
	}
}

func TestSelectedToolPlanBranches(t *testing.T) {
	project := t.TempDir()
	if got := selectedToolPlan(project, "gofumpt"); got.mode != toolBuildIsolated {
		t.Fatalf("generic mode=%q", got.mode)
	}
	if got := selectedToolPlan(project, "protoc-gen-go-lite"); got.mode != toolBuildIsolated {
		t.Fatalf("unselected product mode=%q", got.mode)
	}
}

func TestReconcileToolsStampLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".common-tools-stamp")
	calls := 0
	extract := func() error { calls++; return nil }
	changed, err := reconcileToolsStamp(path, "common@v1", extract)
	if err != nil || !changed || calls != 1 {
		t.Fatalf("first %v %v %d", changed, err, calls)
	}
	changed, err = reconcileToolsStamp(path, "common@v1", extract)
	if err != nil || changed || calls != 1 {
		t.Fatalf("noop %v %v %d", changed, err, calls)
	}
	changed, err = reconcileToolsStamp(path, "common@v2", extract)
	if err != nil || !changed || calls != 2 {
		t.Fatalf("change %v %v %d", changed, err, calls)
	}
	os.Remove(path)
	changed, err = reconcileToolsStamp(path, "common@v3", extract)
	if err != nil || !changed || calls != 3 {
		t.Fatalf("missing %v %v %d", changed, err, calls)
	}
}
func TestReconcileToolsStampFailedExtractionDoesNotAdvance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stamp")
	os.WriteFile(path, []byte("common@old\n"), 0o644)
	_, err := reconcileToolsStamp(path, "common@new", func() error { return errors.New("failed") })
	if err == nil {
		t.Fatal("expected error")
	}
	got, _ := os.ReadFile(path)
	if string(got) != "common@old\n" {
		t.Fatalf("stamp advanced: %q", got)
	}
}
