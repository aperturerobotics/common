package protogen

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/aperturerobotics/protobuf-go-lite/types/descriptorpb"
)

// newTestRustGenerator creates a generator over a temporary project directory.
func newTestRustGenerator(t *testing.T) *RustGenerator {
	t.Helper()

	return &RustGenerator{
		Rust:       &RustConfig{Inventory: DefaultRustInventory},
		ProjectDir: t.TempDir(),
		ModulePath: "github.com/example/app",
	}
}

// writeProjectFile installs one input or previous output in the temporary project.
func writeProjectFile(t *testing.T, g *RustGenerator, file, content string) {
	t.Helper()

	if err := g.writeFile(file, []byte(content)); err != nil {
		t.Fatal(err)
	}
}

func TestRustGeneratorImportPathMapping(t *testing.T) {
	g := newTestRustGenerator(t)

	cases := []struct{ project, canonical string }{
		{"db/block/block.proto", "github.com/example/app/db/block/block.proto"},
		{"vendor/github.com/aperturerobotics/starpc/rpcstream/rpcstream.proto", "github.com/aperturerobotics/starpc/rpcstream/rpcstream.proto"},
	}
	for _, c := range cases {
		if got := g.importPath(c.project); got != c.canonical {
			t.Errorf("importPath(%q) = %q, want %q", c.project, got, c.canonical)
		}
		if got := g.projectPath(c.canonical); got != c.project {
			t.Errorf("projectPath(%q) = %q, want %q", c.canonical, got, c.project)
		}
	}
}

func TestRustGeneratorRenderModulesSplitsPackageAcrossDirectories(t *testing.T) {
	// Schema directories need not match the protobuf package boundary.
	g := newTestRustGenerator(t)

	// Include all outputs belonging to one package in its single Rust module.
	packages := map[string]string{
		"git/git":         "example.git",
		"git/world/world": "example.git",
	}
	got, err := g.renderModules(
		packages,
		[]string{"git/git.pb.rs", "git/git_srpc.pb.rs", "git/world/world.pb.rs"},
	)
	if err != nil {
		t.Fatal(err)
	}

	// Message and service includes share the package's canonical module.
	want := `pub mod example {
    pub mod git {
        include!(concat!(env!("CARGO_MANIFEST_DIR"), "/git/git.pb.rs"));
        include!(concat!(env!("CARGO_MANIFEST_DIR"), "/git/git_srpc.pb.rs"));
        include!(concat!(env!("CARGO_MANIFEST_DIR"), "/git/world/world.pb.rs"));
    }
}
`
	if got != want {
		t.Fatalf("unexpected modules:\n%s\nwant:\n%s", got, want)
	}
}

func TestRustGeneratorRenderModulesRejectsUnknownOutput(t *testing.T) {
	g := newTestRustGenerator(t)

	if _, err := g.renderModules(nil, []string{"stray.pb.rs"}); err == nil {
		t.Fatal("expected an error for an output without a source schema")
	}
}

func TestRustGeneratorSchemaPackagesReadsDescriptorSet(t *testing.T) {
	// Canonical import paths distinguish project files from vendored schemas.
	g := newTestRustGenerator(t)

	// Two files of one package and a dependency, as protoc records them.
	set := marshalDescriptorSet(t,
		descriptorFile("github.com/example/app/git/git.proto", "Example.git"),
		descriptorFile("github.com/example/app/git/world/world.proto", "Example.git"),
		descriptorFile("github.com/dep/x/x.proto", "x"),
		descriptorFile("root.proto", ""),
	)
	got, err := g.schemaPackages(set)
	if err != nil {
		t.Fatal(err)
	}

	// Each descriptor retains its package, including the package-free root.
	want := map[string]string{
		"git/git":                   "Example.git",
		"git/world/world":           "Example.git",
		"vendor/github.com/dep/x/x": "x",
		"vendor/root":               "",
	}
	if !maps.Equal(got, want) {
		t.Fatalf("schemaPackages = %v, want %v", got, want)
	}
}

func TestRustGeneratorSchemaPackagesRejectsTruncatedSet(t *testing.T) {
	g := newTestRustGenerator(t)

	set := marshalDescriptorSet(t, descriptorFile("a.proto", "a"))
	if _, err := g.schemaPackages(set[:len(set)-1]); err == nil {
		t.Fatal("expected an error for a truncated descriptor set")
	}
}

func TestIsRustTypeOption(t *testing.T) {
	cases := map[string]bool{
		"extern_path=.rpcstream=::starpc::rpcstream": true,
		"compile_well_known_types":                   true,
		"compile_well_known_types=false":             true,
		"enable_type_names":                          false,
		"btree_map=.":                                false,
		"boxed=.pkg.Msg.field":                       false,
	}
	for option, want := range cases {
		if got := isRustTypeOption(option); got != want {
			t.Errorf("isRustTypeOption(%q) = %v, want %v", option, got, want)
		}
	}
}

// descriptorFile returns a FileDescriptorProto with a name and, when pkg is not
// empty, a package.
func descriptorFile(name, pkg string) *descriptorpb.FileDescriptorProto {
	file := &descriptorpb.FileDescriptorProto{Name: &name}
	if pkg != "" {
		file.Package = &pkg
	}
	return file
}

// marshalDescriptorSet encodes a FileDescriptorSet of the given files.
func marshalDescriptorSet(t *testing.T, files ...*descriptorpb.FileDescriptorProto) []byte {
	// Attribute encoding failures to the calling contract test.
	t.Helper()

	// Use the same descriptor encoding that protoc emits.
	set := &descriptorpb.FileDescriptorSet{File: files}
	data, err := set.MarshalVT()
	if err != nil {
		t.Fatalf("marshal descriptor set: %v", err)
	}
	return data
}

func TestRustGeneratorPublishWritesChangesAndRemovesObsolete(t *testing.T) {
	// Publication reconciles the previous inventory with the complete new output.
	g := newTestRustGenerator(t)

	// The previous run generated two files and its inventory names both.
	writeProjectFile(t, g, "a/a.pb.rs", "old a")
	writeProjectFile(t, g, "b/b.pb.rs", "obsolete b")
	writeProjectFile(t, g, g.Rust.Inventory, "a/a.pb.rs\nb/b.pb.rs\n")

	// Replace one output, add another and omit the obsolete file.
	outputs := map[string][]byte{
		"a/a.pb.rs":      []byte("new a"),
		"c/c.pb.rs":      []byte("new c"),
		g.Rust.Inventory: []byte("a/a.pb.rs\nc/c.pb.rs\n"),
	}

	// A check reports the drift and leaves the project untouched.
	stale, err := g.publish(outputs, true)
	if err != nil {
		t.Fatal(err)
	}
	wantStale := []string{".protoc-rust-files.txt", "a/a.pb.rs", "c/c.pb.rs", "b/b.pb.rs"}
	if !slices.Equal(stale, wantStale) {
		t.Fatalf("stale = %v, want %v", stale, wantStale)
	}
	if data, _ := os.ReadFile(g.absPath("a/a.pb.rs")); string(data) != "old a" {
		t.Fatalf("check modified a/a.pb.rs: %q", data)
	}
	if _, err := os.Stat(g.absPath("b/b.pb.rs")); err != nil {
		t.Fatalf("check removed b/b.pb.rs: %v", err)
	}

	// Generation applies it.
	if _, err := g.publish(outputs, false); err != nil {
		t.Fatal(err)
	}
	for file, want := range outputs {
		if data, _ := os.ReadFile(g.absPath(file)); string(data) != string(want) {
			t.Fatalf("%s = %q, want %q", file, data, want)
		}
	}
	if _, err := os.Stat(g.absPath("b/b.pb.rs")); !os.IsNotExist(err) {
		t.Fatalf("obsolete b/b.pb.rs remains: %v", err)
	}

	// A repeated run finds nothing to do.
	stale, err = g.publish(outputs, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != 0 {
		t.Fatalf("expected no drift, got %v", stale)
	}
}

func TestRustGeneratorPublishToleratesRemovedObsoleteFile(t *testing.T) {
	g := newTestRustGenerator(t)

	writeProjectFile(t, g, g.Rust.Inventory, "gone/gone.pb.rs\n")

	outputs := map[string][]byte{g.Rust.Inventory: []byte("")}
	if _, err := g.publish(outputs, false); err != nil {
		t.Fatal(err)
	}
}

func TestRustGeneratorPublishRejectsInventoryEscape(t *testing.T) {
	g := newTestRustGenerator(t)

	writeProjectFile(t, g, g.Rust.Inventory, filepath.Join("..", "outside.txt")+"\n")

	outputs := map[string][]byte{g.Rust.Inventory: []byte("")}
	if _, err := g.publish(outputs, false); err == nil {
		t.Fatal("expected an error for an inventory entry outside the project")
	}
}
