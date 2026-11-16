package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/aperturerobotics/cli"
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

func TestToolBinaryCurrentFollowsProjectVersion(t *testing.T) {
	// The test executable records its own dependencies like an installed tool.
	binPath, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	const modulePath = "github.com/aperturerobotics/cli"
	version := builtModuleVersion(binPath, modulePath)
	if version == "" {
		t.Fatalf("test executable records no %s version", modulePath)
	}

	// A matching version reuses the binary; a bumped project version rebuilds it.
	spec := toolSpec{Name: "tool", ModulePath: modulePath}
	current := toolBuildPlan{mode: toolBuildVersioned, spec: spec, version: version}
	if !toolBinaryCurrent(binPath, current) {
		t.Fatal("matching version reported stale")
	}
	bumped := toolBuildPlan{mode: toolBuildVersioned, spec: spec, version: "v99.0.0"}
	if toolBinaryCurrent(binPath, bumped) {
		t.Fatal("bumped version reported current")
	}

	// Isolated tools only require the executable to exist.
	isolated := toolBuildPlan{mode: toolBuildIsolated, spec: spec}
	if !toolBinaryCurrent(binPath, isolated) {
		t.Fatal("isolated tool reported stale")
	}
	if toolBinaryCurrent(filepath.Join(t.TempDir(), "missing"), isolated) {
		t.Fatal("missing tool reported current")
	}
}

func TestReconcileToolsStampLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".common-tools-stamp")
	calls := 0
	extract := func() error { calls++; return nil }
	changed, err := reconcileToolsStamp(path, "common@v1", extract, nil)
	if err != nil || !changed || calls != 1 {
		t.Fatalf("first %v %v %d", changed, err, calls)
	}
	changed, err = reconcileToolsStamp(path, "common@v1", extract, nil)
	if err != nil || changed || calls != 1 {
		t.Fatalf("noop %v %v %d", changed, err, calls)
	}
	changed, err = reconcileToolsStamp(path, "common@v2", extract, nil)
	if err != nil || !changed || calls != 2 {
		t.Fatalf("change %v %v %d", changed, err, calls)
	}
	os.Remove(path)
	changed, err = reconcileToolsStamp(path, "common@v3", extract, nil)
	if err != nil || !changed || calls != 3 {
		t.Fatalf("missing %v %v %d", changed, err, calls)
	}
}

func TestReconcileToolsStampFailedExtractionDoesNotAdvance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stamp")
	os.WriteFile(path, []byte("common@old\n"), 0o644)
	_, err := reconcileToolsStamp(path, "common@new", func() error { return errors.New("failed") }, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	got, _ := os.ReadFile(path)
	if string(got) != "common@old\n" {
		t.Fatalf("stamp advanced: %q", got)
	}
}

func TestInvalidateToolBinariesRemovesCustomGolangCIStamp(t *testing.T) {
	d := t.TempDir()
	bin := filepath.Join(d, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, spec := range defaultTools {
		if err := os.WriteFile(filepath.Join(bin, spec.Name), []byte("old"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// A custom golangci-lint build replaced the stock binary and wrote its
	// stamp; invalidation must drop the stamp with the binary so the next
	// ensureTool run rebuilds the custom build instead of the stock one.
	stamp := filepath.Join(bin, ".golangci-lint-custom-stamp")
	if err := os.WriteFile(stamp, []byte("v1\n/conf"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := invalidateToolBinaries(d); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stamp); !os.IsNotExist(err) {
		t.Fatalf("custom stamp retained: %v", err)
	}
}

func TestInvalidateToolBinariesPreservesUnrelatedFiles(t *testing.T) {
	d := t.TempDir()
	bin := filepath.Join(d, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, spec := range defaultTools {
		if err := os.WriteFile(filepath.Join(bin, spec.Name), []byte("old"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	keep := filepath.Join(bin, "keep")
	if err := os.WriteFile(keep, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := invalidateToolBinaries(d); err != nil {
		t.Fatal(err)
	}
	for _, spec := range defaultTools {
		if _, err := os.Stat(filepath.Join(bin, spec.Name)); !os.IsNotExist(err) {
			t.Fatalf("%s retained", spec.Name)
		}
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("unrelated file removed")
	}
}

func TestReconcileToolsStampDoesNotInvalidateOnMatch(t *testing.T) {
	stamp := filepath.Join(t.TempDir(), "stamp")
	if err := os.WriteFile(stamp, []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := reconcileToolsStamp(stamp, "v1", func() error { t.Fatal("extract called"); return nil }, func() error { t.Fatal("invalidate called"); return nil })
	if err != nil || changed {
		t.Fatalf("matching stamp: changed=%v err=%v", changed, err)
	}
}

// TestFormatPreservesCodeParagraphs keeps comments after continued calls separate.
func TestFormatPreservesCodeParagraphs(t *testing.T) {
	// A continued method call ends before the next action's comment.
	project := t.TempDir()
	path := filepath.Join(project, "paragraph.go")
	const source = `package example

func forward() {
	// Forward the request through the selected client.
	ok, err := newClient().
		InvokeMethod("service", "method")

	// Check the completed request before returning.
	if err != nil {
		panic(err)
	}
	_ = ok
}
`
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}

	// Run the real command twice, without a module or a prepared tool directory.
	app := &cli.App{Name: "aptre", Commands: []*cli.Command{formatCmd}}
	for range 2 {
		if err := app.RunContext(context.Background(), []string{"aptre", "format", "-C", project}); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != source {
			t.Fatalf("formatter changed code paragraph boundaries:\n%s", got)
		}
	}

	// Formatting existing Go sources needs no downloaded project tools.
	if _, err := os.Stat(filepath.Join(project, ".tools")); !os.IsNotExist(err) {
		t.Fatalf("format prepared a tool directory: %v", err)
	}
}
