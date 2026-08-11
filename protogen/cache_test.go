package protogen

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// protocArgsForRoot builds the protoc flag shapes the generator emits for a
// module rooted at root: bare include paths, combined --name=<path> outputs,
// and non-path plugin options.
func protocArgsForRoot(root string) []string {
	vendor := filepath.Join(root, "vendor")
	project := filepath.Join(root, "project")
	return []string{
		"-I", vendor,
		"--proto_path", vendor,
		"-I", filepath.Join(vendor, "github.com", "aperturerobotics", "protobuf", "src"),
		"--go-lite_out=" + vendor,
		"--go-lite_opt=features=marshaler+unmarshaler+size",
		"--go-starpc_out=" + vendor,
		"--es-lite_out=" + vendor,
		"--es-lite_opt=target=ts",
		"--csharp_out=" + project,
	}
}

// TestHashProtocFlagsPortableAcrossCheckouts proves the flag hash is identical
// for two pristine checkouts of the same content at different absolute paths.
func TestHashProtocFlagsPortableAcrossCheckouts(t *testing.T) {
	content := map[string]string{
		"go.mod": "module github.com/aperturerobotics/common\n",
		"vendor/github.com/aperturerobotics/protobuf/src/placeholder.proto": "syntax = \"proto3\";\n",
	}
	rootA, _ := writeProtoTree(t, content)
	rootB, _ := writeProtoTree(t, content)
	if rootA == rootB || !filepath.IsAbs(rootA) || !filepath.IsAbs(rootB) {
		t.Fatalf("test roots must be different absolute directories: A=%q B=%q", rootA, rootB)
	}

	hashA := HashProtocFlags(protocArgsForRoot(rootA), rootA)
	hashB := HashProtocFlags(protocArgsForRoot(rootB), rootB)

	if hashA != hashB {
		t.Fatalf("flag hash must not depend on absolute checkout path:\n  A(%s)=%s\n  B(%s)=%s", rootA, hashA, rootB, hashB)
	}
}

func TestRelativizeProtocFlag(t *testing.T) {
	root := t.TempDir()
	vendor := filepath.Join(root, "vendor")

	tests := []struct {
		name string
		flag string
		want string
	}{
		{
			name: "separate path argument",
			flag: vendor,
			want: "vendor",
		},
		{
			name: "path flag",
			flag: "--go-lite_out=" + vendor,
			want: "--go-lite_out=vendor",
		},
		{
			name: "multiple equals non-path value",
			flag: "--go-lite_opt=features=marshaler+unmarshaler+size",
			want: "--go-lite_opt=features=marshaler+unmarshaler+size",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := relativizeProtocFlag(test.flag, root); got != test.want {
				t.Fatalf("relativizeProtocFlag(%q) = %q, want %q", test.flag, got, test.want)
			}
		})
	}
}

// TestHashProtocFlagsChangesOnGeneratorRelevantChange proves the hash still
// changes when a plugin option, the output layout, or the enabled plugin set
// changes, so caches are not incorrectly reused.
func TestHashProtocFlagsChangesOnGeneratorRelevantChange(t *testing.T) {
	root := filepath.Join("/tmp", "wt", "common")
	base := HashProtocFlags(protocArgsForRoot(root), root)

	t.Run("plugin option value", func(t *testing.T) {
		changed := protocArgsForRoot(root)
		for i, a := range changed {
			if a == "--go-lite_opt=features=marshaler+unmarshaler+size" {
				changed[i] = "--go-lite_opt=features=marshaler"
			}
		}
		if h := HashProtocFlags(changed, root); h == base {
			t.Fatalf("hash must change when a plugin option changes")
		}
	})

	t.Run("output layout", func(t *testing.T) {
		// Move the go-lite output from vendor/ to a different module-relative
		// directory; the relative suffix differs, so the hash must differ.
		changed := protocArgsForRoot(root)
		for i, a := range changed {
			if a == "--go-lite_out="+filepath.Join(root, "vendor") {
				changed[i] = "--go-lite_out=" + filepath.Join(root, "gen")
			}
		}
		if h := HashProtocFlags(changed, root); h == base {
			t.Fatalf("hash must change when the output layout changes")
		}
	})

	t.Run("added plugin flag", func(t *testing.T) {
		changed := append(protocArgsForRoot(root), "--prost_out="+filepath.Join(root, "vendor"))
		if h := HashProtocFlags(changed, root); h == base {
			t.Fatalf("hash must change when a plugin is enabled")
		}
	})
}

// TestRelativizePathKeepsOutOfTreePaths documents the STOP-POINT boundary: an
// absolute path outside the module root is not our normalization domain and is
// preserved verbatim rather than rewritten to a "../" escape.
func TestRelativizePathKeepsOutOfTreePaths(t *testing.T) {
	root := filepath.Join("/tmp", "wt", "common")
	outside := filepath.Join("/opt", "external", "protos")
	if got := relativizePath(outside, root); got != outside {
		t.Fatalf("out-of-tree path must be preserved: got %q want %q", got, outside)
	}
	// A sibling directory sharing a prefix must not be partially rewritten.
	sibling := root + "-other"
	if got := relativizePath(sibling, root); got != sibling {
		t.Fatalf("prefix-sharing sibling must be preserved: got %q want %q", got, sibling)
	}
}

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

// TestHashProtoFilesPortableAndContentSensitive proves the per-package content
// hash is identical for the same proto content at different absolute project
// paths, yet changes when a proto source is mutated.
func TestHashProtoFilesPortableAndContentSensitive(t *testing.T) {
	content := map[string]string{
		"foo/bar.proto": "syntax = \"proto3\";\npackage foo;\nmessage Bar { string id = 1; }\n",
	}

	dirA, filesA := writeProtoTree(t, content)
	dirB, filesB := writeProtoTree(t, content)

	hashA, err := hashProtoFiles(filesA, dirA)
	if err != nil {
		t.Fatalf("hashProtoFiles A: %v", err)
	}
	hashB, err := hashProtoFiles(filesB, dirB)
	if err != nil {
		t.Fatalf("hashProtoFiles B: %v", err)
	}
	if hashA != hashB {
		t.Fatalf("content hash must not depend on absolute project path: A=%s B=%s", hashA, hashB)
	}

	mutated := map[string]string{
		"foo/bar.proto": "syntax = \"proto3\";\npackage foo;\nmessage Bar { string id = 1; int32 n = 2; }\n",
	}
	dirC, filesC := writeProtoTree(t, mutated)
	hashC, err := hashProtoFiles(filesC, dirC)
	if err != nil {
		t.Fatalf("hashProtoFiles C: %v", err)
	}
	if hashC == hashA {
		t.Fatalf("content hash must change when a proto source changes")
	}
}

func TestToolVersionChangeInvalidatesCachedPackage(t *testing.T) {
	dir, files := writeProtoTree(t, map[string]string{
		"foo.proto": "syntax = \"proto3\";\npackage foo;\n",
	})
	cache := NewCache()
	cache.ProtocFlagsHash = "flags"
	cache.ToolVersions = "starpc-python=old"
	if err := cache.UpdatePackage("example/foo", files, nil, dir); err != nil {
		t.Fatal(err)
	}

	stale, err := cache.NeedsRegeneration(
		"example/foo", files, dir, "flags", "starpc-python=old", false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if stale {
		t.Fatal("matching tool state invalidated cached output")
	}
	stale, err = cache.NeedsRegeneration(
		"example/foo", files, dir, "flags", "starpc-python=new", false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !stale {
		t.Fatal("changed tool state reused cached output")
	}
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

func TestGenerateFailureDoesNotPersistToolVersions(t *testing.T) {
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
	cache := NewCache()
	cache.ToolVersions = "accepted"
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
		Cache:      cache,
		ProjectDir: dir,
		ModuleDir:  dir,
		ModulePath: "example.com/project",
		VendorDir:  vendorDir,
		OutDir:     vendorDir,
		Stdout:     io.Discard,
		Stderr:     io.Discard,
	}
	if err := generator.Generate(context.Background()); err == nil {
		t.Fatal("invalid source generation unexpectedly succeeded")
	}
	if cache.ToolVersions != "accepted" {
		t.Fatalf("failed generation persisted tool state %q", cache.ToolVersions)
	}
}
