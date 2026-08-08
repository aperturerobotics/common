package protogen

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestDiscoverPluginsStarpcPythonSelected(t *testing.T) {
	projectDir := newPluginTestProject(t, false)
	cfg := NewConfig()
	cfg.ProjectDir = projectDir
	cfg.Languages = []string{"python"}
	cfg.RPCLibraries = []string{"starpc-python"}

	plugins, err := DiscoverPlugins(cfg)
	if err != nil {
		t.Fatalf("discover plugins: %v", err)
	}
	if plugins.StarpcPython == nil {
		t.Fatal("expected starpc-python plugin")
	}
	if got := plugins.GetProtocArgs("/out", "/csharp"); !slices.Contains(got, "--starpc-python_out=/out") {
		t.Fatalf("expected starpc-python arg, got %v", got)
	}
	h := NewNativePluginHandler(plugins, false)
	if got := h.findPluginPath("protoc-gen-starpc-python", false); got != plugins.StarpcPython.Path {
		t.Fatalf("handler path = %q, want %q", got, plugins.StarpcPython.Path)
	}
}

func TestDiscoverPluginsStarpcPythonMissingBinaryFails(t *testing.T) {
	projectDir := newPluginTestProject(t, false)
	if err := os.Remove(filepath.Join(projectDir, ".tools", "bin", "protoc-gen-starpc-python")); err != nil {
		t.Fatalf("remove test plugin: %v", err)
	}
	cfg := NewConfig()
	cfg.ProjectDir = projectDir
	cfg.Languages = []string{"python"}
	cfg.RPCLibraries = []string{"starpc-python"}
	if _, err := DiscoverPlugins(cfg); err == nil || !strings.Contains(err.Error(), "starpc-python selected") {
		t.Fatalf("expected clear missing-plugin error, got %v", err)
	}
}

func TestDiscoverPluginsStarpcPythonUnselected(t *testing.T) {
	projectDir := newPluginTestProject(t, false)
	cfg := NewConfig()
	cfg.ProjectDir = projectDir
	cfg.Languages = []string{"python"}
	cfg.RPCLibraries = []string{"none"}
	plugins, err := DiscoverPlugins(cfg)
	if err != nil {
		t.Fatalf("discover plugins: %v", err)
	}
	if plugins.StarpcPython != nil {
		t.Fatal("expected no starpc-python plugin")
	}
	if slices.Contains(plugins.GetProtocArgs("/out", "/csharp"), "--starpc-python_out=/out") {
		t.Fatal("unexpected starpc-python arg")
	}
}

func TestDiscoverPluginsDefaultAllLanguages(t *testing.T) {
	t.Helper()

	projectDir := newPluginTestProject(t, true)
	cfg := NewConfig()
	cfg.ProjectDir = projectDir

	plugins, err := DiscoverPlugins(cfg)
	if err != nil {
		t.Fatalf("discover plugins: %v", err)
	}
	if plugins.GoLite == nil {
		t.Fatal("expected go-lite plugin")
	}
	if plugins.GoStarpc == nil {
		t.Fatal("expected go-starpc plugin")
	}
	if plugins.ESLite == nil {
		t.Fatal("expected es-lite plugin")
	}
	if plugins.ESStarpc == nil {
		t.Fatal("expected es-starpc plugin")
	}
	if plugins.CppStarpc == nil {
		t.Fatal("expected starpc-cpp plugin")
	}
	if plugins.RustProst == nil {
		t.Fatal("expected prost plugin")
	}
	if plugins.RustStarpc == nil {
		t.Fatal("expected starpc-rust plugin")
	}

	args := plugins.GetProtocArgs("/out", "/csharp")
	for _, want := range []string{
		"--cpp_out=/out",
		"--go-lite_out=/out",
		"--go-starpc_out=/out",
		"--es-lite_out=/out",
		"--es-starpc_out=/out",
		"--starpc-cpp_out=/out",
		"--prost_out=/out",
		"--starpc-rust_out=/out",
	} {
		if !slices.Contains(args, want) {
			t.Fatalf("expected protoc arg %q in %v", want, args)
		}
	}
	for _, unwanted := range []string{"--csharp_out=/out", "--python_out=/out"} {
		if slices.Contains(args, unwanted) {
			t.Fatalf("default language set unexpectedly contains %q in %v", unwanted, args)
		}
	}
}

func TestDiscoverPluginsGoLanguageOnly(t *testing.T) {
	t.Helper()

	projectDir := newPluginTestProject(t, true)
	cfg := NewConfig()
	cfg.ProjectDir = projectDir
	cfg.Languages = []string{"go"}

	plugins, err := DiscoverPlugins(cfg)
	if err != nil {
		t.Fatalf("discover plugins: %v", err)
	}
	if plugins.GoLite == nil {
		t.Fatal("expected go-lite plugin")
	}
	if plugins.GoStarpc == nil {
		t.Fatal("expected go-starpc plugin")
	}
	if plugins.ESLite != nil || plugins.ESStarpc != nil {
		t.Fatal("expected no TypeScript plugins")
	}
	if plugins.CppStarpc != nil {
		t.Fatal("expected no C++ starpc plugin")
	}
	if plugins.RustProst != nil || plugins.RustStarpc != nil {
		t.Fatal("expected no Rust plugins")
	}

	args := plugins.GetProtocArgs("/out", "/csharp")
	for _, arg := range args {
		if strings.Contains(arg, "cpp") {
			t.Fatalf("expected no C++ args, got %v", args)
		}
		if strings.Contains(arg, "prost") || strings.Contains(arg, "rust") {
			t.Fatalf("expected no Rust args, got %v", args)
		}
		if strings.Contains(arg, "es-") {
			t.Fatalf("expected no TypeScript args, got %v", args)
		}
	}
	for _, want := range []string{"--go-lite_out=/out", "--go-starpc_out=/out"} {
		if !slices.Contains(args, want) {
			t.Fatalf("expected protoc arg %q in %v", want, args)
		}
	}
}

func TestDiscoverPluginsGoLanguageNoRPC(t *testing.T) {
	t.Helper()

	projectDir := newPluginTestProject(t, true)
	cfg := NewConfig()
	cfg.ProjectDir = projectDir
	cfg.Languages = []string{"go"}
	cfg.RPCLibraries = []string{"none"}

	plugins, err := DiscoverPlugins(cfg)
	if err != nil {
		t.Fatalf("discover plugins: %v", err)
	}
	if plugins.GoLite == nil {
		t.Fatal("expected go-lite plugin")
	}
	if plugins.GoStarpc != nil {
		t.Fatal("expected no go-starpc plugin")
	}

	args := plugins.GetProtocArgs("/out", "/csharp")
	if !slices.Contains(args, "--go-lite_out=/out") {
		t.Fatalf("expected go-lite protoc arg in %v", args)
	}
	for _, arg := range args {
		if strings.Contains(arg, "starpc") {
			t.Fatalf("expected no StarPC args, got %v", args)
		}
	}
}

func TestDiscoverPluginsRustLanguageNoRPC(t *testing.T) {
	t.Helper()

	projectDir := newPluginTestProject(t, true)
	cfg := NewConfig()
	cfg.ProjectDir = projectDir
	cfg.Languages = []string{"rust"}
	cfg.RPCLibraries = []string{"false"}

	plugins, err := DiscoverPlugins(cfg)
	if err != nil {
		t.Fatalf("discover plugins: %v", err)
	}
	if plugins.RustProst == nil {
		t.Fatal("expected prost plugin")
	}
	if plugins.RustStarpc != nil {
		t.Fatal("expected no rust starpc plugin")
	}

	args := plugins.GetProtocArgs("/out", "/csharp")
	if !slices.Contains(args, "--prost_out=/out") {
		t.Fatalf("expected prost protoc arg in %v", args)
	}
	for _, arg := range args {
		if strings.Contains(arg, "starpc") {
			t.Fatalf("expected no StarPC args, got %v", args)
		}
	}
}

func TestDiscoverPluginsLanguagePresenceGate(t *testing.T) {
	t.Helper()

	projectDir := newPluginTestProject(t, false)
	cfg := NewConfig()
	cfg.ProjectDir = projectDir
	cfg.Languages = []string{"ts"}

	plugins, err := DiscoverPlugins(cfg)
	if err != nil {
		t.Fatalf("discover plugins: %v", err)
	}
	if plugins.GoLite != nil || plugins.GoStarpc != nil {
		t.Fatal("expected no Go plugins")
	}
	if plugins.ESLite != nil || plugins.ESStarpc != nil {
		t.Fatal("expected no TypeScript plugins without package.json")
	}
	if plugins.CppStarpc != nil {
		t.Fatal("expected no C++ plugin")
	}
	if plugins.RustProst != nil || plugins.RustStarpc != nil {
		t.Fatal("expected no Rust plugins")
	}
	if args := plugins.GetProtocArgs("/out", "/csharp"); len(args) != 0 {
		t.Fatalf("expected no protoc args, got %v", args)
	}
}

func TestDiscoverPluginsCSharpAndPython(t *testing.T) {
	t.Helper()

	projectDir := newPluginTestProject(t, false)
	cfg := NewConfig()
	cfg.ProjectDir = projectDir
	cfg.Languages = []string{"csharp", "python"}

	plugins, err := DiscoverPlugins(cfg)
	if err != nil {
		t.Fatalf("discover plugins: %v", err)
	}
	want := []string{"--csharp_out=/csharp", "--python_out=/out", "--pyi_out=/out"}
	if args := plugins.GetProtocArgs("/out", "/csharp"); !slices.Equal(args, want) {
		t.Fatalf("expected protoc args %v, got %v", want, args)
	}
}

func TestDiscoverPluginsUnknownLanguage(t *testing.T) {
	t.Helper()

	projectDir := newPluginTestProject(t, true)
	cfg := NewConfig()
	cfg.ProjectDir = projectDir
	cfg.Languages = []string{"go", "kotlin"}

	if _, err := DiscoverPlugins(cfg); err == nil {
		t.Fatal("expected unknown language error")
	}
}

func newPluginTestProject(t *testing.T, withPackageJSON bool) string {
	t.Helper()

	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module example.com/test\n\ngo 1.25.0\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if withPackageJSON {
		if err := os.WriteFile(filepath.Join(projectDir, "package.json"), []byte(`{"name":"test"}`), 0o644); err != nil {
			t.Fatalf("write package.json: %v", err)
		}
	}

	for _, name := range []string{
		"protoc-gen-go-lite",
		"protoc-gen-go-starpc",
		"protoc-gen-starpc-cpp",
		"protoc-gen-starpc-rust",
		"protoc-gen-prost",
		"protoc-gen-starpc-python",
	} {
		writeTestFile(t, filepath.Join(projectDir, ".tools", "bin", name))
	}
	for _, name := range []string{
		"protoc-gen-es-lite",
		"protoc-gen-es-starpc",
	} {
		writeTestFile(t, filepath.Join(projectDir, "node_modules", ".bin", name))
	}

	return projectDir
}

func writeTestFile(t *testing.T, name string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(name), err)
	}
	if err := os.WriteFile(name, []byte(""), 0o755); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestDiscoverNodePluginPackageBinFallbackAndPrecedence(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module example.com/test\n\ngo 1.25.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(projectDir, "cmd", "protoc-gen-es-starpc")
	writeTestFile(t, bin)
	os.WriteFile(filepath.Join(projectDir, "package.json"), []byte(`{"bin":{"protoc-gen-es-starpc":"./cmd/protoc-gen-es-starpc"}}`), 0o644)
	cfg := NewConfig()
	cfg.ProjectDir = projectDir
	cfg.Languages = []string{"ts"}
	cfg.RPCLibraries = []string{"starpc"}
	plugins, err := DiscoverPlugins(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if plugins.ESStarpc == nil || plugins.ESStarpc.Path != bin {
		t.Fatalf("fallback path=%v", plugins.ESStarpc)
	}
	installed := filepath.Join(projectDir, "node_modules", ".bin", "protoc-gen-es-starpc")
	writeTestFile(t, installed)
	plugins, err = DiscoverPlugins(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if plugins.ESStarpc.Path != installed {
		t.Fatalf("installed precedence=%q", plugins.ESStarpc.Path)
	}
}
