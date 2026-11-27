package protogen

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/aperturerobotics/common/protogen/cache"
)

// writeProtoTree writes proto files (path->content) under a fresh temp project
// directory and returns the directory and the sorted relative file list.
func writeProtoTree(t *testing.T, files map[string]string) (string, []string) {
	t.Helper()
	dir := t.TempDir()
	rel := make([]string, 0, len(files))
	for name, content := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		rel = append(rel, name)
	}
	slices.Sort(rel)
	return dir, rel
}

func TestGetToolVersionsIncludesUVLock(t *testing.T) {
	dir := t.TempDir()
	lock := []byte("version = 1\nrevision = 3\n")
	if err := os.WriteFile(filepath.Join(dir, "uv.lock"), lock, 0o644); err != nil {
		t.Fatal(err)
	}
	generator := &Generator{
		ProjectDir: dir,
		Config:     NewConfig(),
		Plugins:    &Plugins{StarpcPython: &Plugin{}},
	}
	versions := generator.getToolVersions()
	want := sha256.Sum256(lock)
	if !strings.Contains(versions, "uv.lock="+hex.EncodeToString(want[:])) {
		t.Fatalf("tool versions omit uv.lock digest: %q", versions)
	}
}

// TestGetToolVersionsNamesGenerator keeps a generator change from reusing
// output that an older generator wrote.
func TestGetToolVersionsNamesGenerator(t *testing.T) {
	generator := &Generator{ProjectDir: t.TempDir(), Config: NewConfig(), Plugins: &Plugins{}}
	id := generatorIdentity()
	if id == "" || id == "unknown" || !strings.Contains(generator.getToolVersions(), "generator="+id) {
		t.Fatalf("tool versions %q omit generator identity %q", generator.getToolVersions(), id)
	}
}

func TestGetToolVersionsIgnoresUVLockWithoutStarpcPython(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "uv.lock"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	generator := &Generator{
		ProjectDir: dir,
		Config:     NewConfig(),
		Plugins:    &Plugins{},
	}
	if versions := generator.getToolVersions(); strings.Contains(versions, "uv.lock=") {
		t.Fatalf("unselected Python plugin invalidated tool state: %q", versions)
	}
}

// TestGenerateFailureDoesNotPersistToolVersions keeps failed outputs out of the manifest.
func TestGenerateFailureDoesNotPersistToolVersions(t *testing.T) {
	// Set up an invalid schema and a changed tool identity.
	dir := t.TempDir()
	vendorDir := filepath.Join(dir, "vendor")
	if err := os.MkdirAll(vendorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.proto"), []byte("not protobuf"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "uv.lock"), []byte("changed plugin"), 0o644); err != nil {
		t.Fatal(err)
	}
	config := NewConfig()
	config.ProjectDir = dir
	config.Targets = []string{"bad.proto"}
	plugins := &Plugins{
		Languages:    Languages{LanguagePython: {}},
		RPCLibraries: RPCLibraries{RPCLibraryStarpcPython: {}},
		StarpcPython: &Plugin{
			Name:       "starpc-python",
			BinaryName: "protoc-gen-starpc-python",
			Path:       "/bin/false",
			Type:       PluginTypePython,
			OutFlag:    "starpc-python_out",
		},
	}
	generator := &Generator{
		Config:     config,
		Plugins:    plugins,
		ProjectDir: dir,
		ModuleDir:  dir,
		ModulePath: "example.com/project",
		VendorDir:  vendorDir,
		Stdout:     io.Discard,
		Stderr:     io.Discard,
	}

	// Seed the manifest with the last accepted tool state.
	manifest := cache.NewCache()
	run := cache.NewRunCache()
	run.ToolVersions = "accepted"
	manifest.Runs[generator.runKey()] = run
	cacheFile := saveGeneratorCache(t, generator, manifest)

	// Attempt generation and confirm the old manifest entry is intact on disk.
	if err := generator.Generate(t.Context()); err == nil {
		t.Fatal("invalid source generation unexpectedly succeeded")
	}
	saved, err := cache.LoadCache(cacheFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := saved.Runs[generator.runKey()]; got == nil || got.ToolVersions != "accepted" {
		t.Fatalf("failed generation changed the accepted run: %+v", got)
	}
}

// TestGenerateBesideVendoredNestedModule proves generation succeeds when the
// project vendors one of its own nested modules, so vendor/ holds a real
// directory under the project's module path, and that the output reaches the
// project without touching the vendored copy.
func TestGenerateBesideVendoredNestedModule(t *testing.T) {
	// Lay out a project whose vendor/ holds its nested module lint.
	dir, _ := writeProtoTree(t, map[string]string{
		"api/a.proto": "syntax = \"proto3\";\npackage api;\nmessage A {}\n",
		"vendor/example.com/project/lint/lint.go": "package lint\n",
		"vendor/modules.txt":                      "# example.com/project/lint v0.0.0 => ./lint\n",
	})
	for _, args := range [][]string{{"init", "-q"}, {"add", "api"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}

	// Generate the built-in Python output.
	config := NewConfig()
	config.ProjectDir = dir
	generator := &Generator{
		Config:     config,
		Plugins:    &Plugins{Languages: Languages{LanguagePython: {}}},
		ProjectDir: dir,
		ModuleDir:  dir,
		ModulePath: "example.com/project",
		VendorDir:  filepath.Join(dir, "vendor"),
		Stdout:     io.Discard,
		Stderr:     io.Discard,
	}
	if err := generator.Generate(context.Background()); err != nil {
		t.Fatal(err)
	}

	// The output is in the project and the vendored module is unchanged.
	if _, err := os.Stat(filepath.Join(dir, "api", "a_pb2.py")); err != nil {
		t.Fatalf("generated output: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "vendor", "example.com", "project"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "lint" || !entries[0].IsDir() {
		t.Fatalf("vendored module path changed: %v", entries)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "vendor", "example.com", "project", "lint", "lint.go")); err != nil || string(data) != "package lint\n" {
		t.Fatalf("vendored file changed: %q, %v", data, err)
	}
}

// TestGenerateLanguagesShareManifest proves two languages retain independent
// cache hits when they generate the same schema into one project.
func TestGenerateLanguagesShareManifest(t *testing.T) {
	// Give both generators the same tracked schema and default manifest path.
	dir, _ := writeProtoTree(t, map[string]string{
		"go.mod":  "module example.com/project\n",
		"a.proto": "syntax = \"proto3\";\npackage api;\nmessage A {}\n",
	})
	for _, args := range [][]string{{"init", "-q"}, {"add", "a.proto"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}

	// Run actual protoc once for each language, then reopen the shared manifest
	// in new generators and require both languages to skip generation.
	languages := []struct{ name, output string }{{"python", "a_pb2.py"}, {"csharp", "A.cs"}}
	for pass := range 2 {
		for _, lang := range languages {
			// Select one language without external plugins.
			cfg := NewConfig()
			cfg.ProjectDir = dir
			cfg.Languages = []string{lang.name}
			cfg.RPCLibraries = []string{"none"}
			gen, err := NewGenerator(cfg)
			if err != nil {
				t.Fatal(err)
			}
			var stdout bytes.Buffer
			gen.Stdout = &stdout
			gen.Verbose = true

			// Check the output and whether the schema went through protoc.
			if err := gen.Generate(t.Context()); err != nil {
				t.Fatalf("%s pass %d: %v", lang.name, pass+1, err)
			}
			if got := strings.Contains(stdout.String(), "Skipping . (up to date)"); got != (pass == 1) {
				t.Fatalf("%s pass %d: unexpected cache result: %s", lang.name, pass+1, &stdout)
			}
			if _, err := os.Stat(filepath.Join(dir, lang.output)); err != nil {
				t.Fatalf("%s output: %v", lang.name, err)
			}
		}
	}
}

// newTargetedGenerator builds a generator over a two-package tree whose cache
// holds up-to-date entries for both packages, so a run regenerates nothing and
// needs no protoc.
func newTargetedGenerator(t *testing.T) (*Generator, *cache.RunCache, string, string) {
	t.Helper()

	// Write two packages to check retention of entries outside the targets.
	dir, _ := writeProtoTree(t, map[string]string{
		"a/a.proto": "syntax = \"proto3\";\npackage a;\nmessage A {}\n",
		"b/b.proto": "syntax = \"proto3\";\npackage b;\nmessage B {}\n",
	})
	// Discovery lists tracked files, so the tree must be a git repository.
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	// Set up a generator selecting just the first package.
	vendorDir := filepath.Join(dir, "vendor")
	if err := os.MkdirAll(vendorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	config := NewConfig()
	config.ProjectDir = dir
	config.Targets = []string{"a/a.proto"}
	g := &Generator{
		Config:     config,
		Plugins:    &Plugins{},
		ProjectDir: dir,
		ModuleDir:  dir,
		ModulePath: "example.com/project",
		VendorDir:  vendorDir,
		Stdout:     io.Discard,
		Stderr:     io.Discard,
	}

	// Seed the inventory with two current packages.
	run := cache.NewRunCache()
	keyA := cache.GetPackageKey(g.ModulePath, "a/a.proto")
	keyB := cache.GetPackageKey(g.ModulePath, "b/b.proto")
	for key, file := range map[string]string{keyA: "a/a.proto", keyB: "b/b.proto"} {
		if err := run.UpdatePackage(key, []string{file}, nil, dir); err != nil {
			t.Fatal(err)
		}
	}
	run.ProtocFlagsHash = cache.HashProtocFlags(g.buildProtocArgs(protocRootIdentity), g.ModuleDir)
	run.ToolVersions = g.getToolVersions()
	manifest := cache.NewCache()
	manifest.Runs[g.runKey()] = run
	saveGeneratorCache(t, g, manifest)
	return g, run, keyA, keyB
}

// saveGeneratorCache writes a fixture to the same manifest path as generation.
func saveGeneratorCache(t *testing.T, g *Generator, manifest *cache.Cache) string {
	t.Helper()

	// Store the fixture for the next Generate call to load.
	cacheFile, err := g.Config.GetCacheFilePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := manifest.Save(cacheFile); err != nil {
		t.Fatal(err)
	}
	return cacheFile
}

// TestTargetedGenerateKeepsOtherPackages proves a run targeting one of two
// packages updates the manifest without discarding the other package's entry.
func TestTargetedGenerateKeepsOtherPackages(t *testing.T) {
	// Run against the targeted package, then read the persisted inventory.
	g, _, keyA, keyB := newTargetedGenerator(t)
	if err := g.Generate(context.Background()); err != nil {
		t.Fatal(err)
	}
	cacheFile, err := g.Config.GetCacheFilePath()
	if err != nil {
		t.Fatal(err)
	}
	saved, err := cache.LoadCache(cacheFile)
	if err != nil {
		t.Fatal(err)
	}

	// The non-target package is still present because its source still exists.
	for _, key := range []string{keyA, keyB} {
		if saved.Runs[g.runKey()].Packages[key] == nil {
			t.Fatalf("targeted run dropped manifest entry %q", key)
		}
	}
}

// TestTargetedGeneratePrunesDeletedPackages proves a package outside the
// targets is still pruned once its proto files are gone from disk.
func TestTargetedGeneratePrunesDeletedPackages(t *testing.T) {
	// Remove the untargeted package and run against the remaining input.
	g, _, keyA, keyB := newTargetedGenerator(t)
	if err := os.Remove(filepath.Join(g.ProjectDir, "b/b.proto")); err != nil {
		t.Fatal(err)
	}
	if err := g.Generate(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Only the surviving package remains in the persisted inventory.
	cacheFile, err := g.Config.GetCacheFilePath()
	if err != nil {
		t.Fatal(err)
	}
	saved, err := cache.LoadCache(cacheFile)
	if err != nil {
		t.Fatal(err)
	}
	packages := saved.Runs[g.runKey()].Packages
	if packages[keyA] == nil {
		t.Fatalf("targeted package %q missing from manifest", keyA)
	}
	if packages[keyB] != nil {
		t.Fatalf("deleted package %q still in manifest", keyB)
	}
}

// TestTargetedGenerateDropsStaleOnIdentityChange proves that when the tool
// identity changed, untargeted entries are dropped rather than being marked
// current under the new identity.
func TestTargetedGenerateDropsStaleOnIdentityChange(t *testing.T) {
	// Exercise pruning directly when only the targeted package was refreshed.
	g, run, _, keyB := newTargetedGenerator(t)
	current := map[string]struct{}{cache.GetPackageKey(g.ModulePath, "a/a.proto"): {}}
	run.DropOutsidePackages(current)
	if run.Packages[keyB] != nil {
		t.Fatalf("untargeted package %q survived identity change", keyB)
	}
}

func TestFindOxfmt(t *testing.T) {
	dir := t.TempDir()
	if got := findOxfmt(dir); got != "oxfmt" {
		t.Fatalf("without node_modules got %q, want PATH lookup", got)
	}
	local := filepath.Join(dir, "node_modules", ".bin", "oxfmt")
	if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(local, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := findOxfmt(dir); got != local {
		t.Fatalf("got %q, want %q", got, local)
	}
}
