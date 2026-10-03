package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aperturerobotics/cli"
)

// copyRustGraphProject copies the tests/rust-graph project into a temporary
// directory and tracks its schemas in a new Git repository.
func copyRustGraphProject(t *testing.T) string {
	// Attribute fixture failures to the contract that requested the project.
	t.Helper()

	// Copy the project, leaving out the output of its consumer crate.
	source := filepath.Join(repoRoot(t), "tests", "rust-graph")
	projectDir := t.TempDir()
	err := filepath.WalkDir(source, func(file string, entry fs.DirEntry, err error) error {
		// Omit consumer build products from the isolated command input.
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, file)
		if err != nil {
			return err
		}
		if rel == "target" || rel == ".tools" {
			return fs.SkipDir
		}

		// Preserve the fixture's source tree at the temporary project root.
		target := filepath.Join(projectDir, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copy project: %v", err)
	}

	// Discovery consumes tracked schemas, just as it does in a real project.
	runTestCommand(t, projectDir, "git", "init")
	runTestCommand(t, projectDir, "git", "add", "package.json", "core", "ext", "legacy", "shared", "svc")
	return projectDir
}

// runRustGraph runs the public aptre generate command over projectDir with its
// default dependency preparation.
func runRustGraph(t *testing.T, projectDir string, args ...string) error {
	t.Helper()

	app := &cli.App{Commands: []*cli.Command{generateCmd}}
	argv := append([]string{"aptre", "generate", "-C", projectDir}, args...)
	return app.Run(argv)
}

// readRustGraphFile reads a generated file of the project.
func readRustGraphFile(t *testing.T, projectDir, file string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(projectDir, filepath.FromSlash(file)))
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	return string(data)
}

// TestGenerateRustCheckNeverPreparesDependencies runs the default read-only
// command in a project whose service plugin is not built. The command must
// report the missing plugin instead of building it, and must leave no tools
// directory behind.
func TestGenerateRustCheckNeverPreparesDependencies(t *testing.T) {
	// Leave the service plugin absent from an otherwise valid project.
	projectDir := copyRustGraphProject(t)

	// Read-only checking must report the missing tool without installing it.
	err := runRustGraph(t, projectDir, "--check")
	if err == nil {
		t.Fatal("expected the check to require the service plugin")
	}
	if !strings.Contains(err.Error(), "protoc-gen-starpc-rust is not built") {
		t.Fatalf("check error does not name the missing plugin: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, ".tools")); !os.IsNotExist(err) {
		t.Fatalf("check prepared the tools directory: %v", err)
	}
}

// TestGenerateRustWholeGraphMessages runs the default command over a project
// with a package spread over directories, two service files in one package, a
// renamed package, and exact and prefix extern paths. It generates the message
// files only, which needs no built plugin; tests/rust-graph/check.bash compiles
// the same project with its services.
func TestGenerateRustWholeGraphMessages(t *testing.T) {
	// Use the complete consumer schema graph through the public command.
	projectDir := copyRustGraphProject(t)

	// Message-only generation uses the embedded plugin without a tool install.
	if err := runRustGraph(t, projectDir, "--rpc", "none"); err != nil {
		t.Fatalf("generate: %v", err)
	}

	// Every schema yields its own file beside it; the extern-only root has none.
	for _, file := range []string{
		"core/core.pb.rs", "core/extra/extra.pb.rs", "shared/shared.pb.rs",
		"legacy/legacy.pb.rs", "svc/request.pb.rs", "modules.rs", "descriptors.bin",
		"generated-files.txt",
	} {
		if _, err := os.Stat(filepath.Join(projectDir, filepath.FromSlash(file))); err != nil {
			t.Errorf("missing output %s: %v", file, err)
		}
	}
	if _, err := os.Stat(filepath.Join(projectDir, "ext", "ext.pb.rs")); !os.IsNotExist(err) {
		t.Errorf("excluded schema generated output: %v", err)
	}

	// A schema that declares only services has no message file to include.
	if _, err := os.Stat(filepath.Join(projectDir, "svc", "graph.pb.rs")); !os.IsNotExist(err) {
		t.Errorf("service-only schema generated a message file: %v", err)
	}

	// Both files of the shared package sit in one module, and the renamed
	// package is nested under the names Prost gives it.
	modules := readRustGraphFile(t, projectDir, "modules.rs")
	for _, want := range []string{
		"pub mod core {",
		`"/core/core.pb.rs"`,
		`"/core/extra/extra.pb.rs"`,
		"pub mod legacy {",
		"pub mod r#type {",
	} {
		if !strings.Contains(modules, want) {
			t.Errorf("modules.rs lacks %q:\n%s", want, modules)
		}
	}
	if strings.Count(modules, "pub mod core {") != 1 {
		t.Errorf("package graph.core is declared more than once:\n%s", modules)
	}

	// A second run finds nothing outdated, and a check reports edited output.
	if err := runRustGraph(t, projectDir, "--rpc", "none", "--check"); err != nil {
		t.Fatalf("check after generate: %v", err)
	}
	edited := filepath.Join(projectDir, "core", "core.pb.rs")
	if err := os.WriteFile(edited, []byte("// edited\n"), 0o644); err != nil {
		t.Fatalf("edit output: %v", err)
	}
	if err := runRustGraph(t, projectDir, "--rpc", "none", "--check"); err == nil {
		t.Fatal("expected the check to report the edited output")
	}
}
