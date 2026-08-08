package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/aperturerobotics/cli"

	"github.com/aperturerobotics/common/protogen"
)

func TestGenerateLanguageFlagAliasSharesConfigField(t *testing.T) {
	t.Helper()

	var languageFlag *cli.StringSliceFlag
	for _, flag := range generateCmd.Flags {
		if candidate, ok := flag.(*cli.StringSliceFlag); ok && candidate.Name == "language" {
			languageFlag = candidate
			break
		}
	}
	if languageFlag == nil {
		t.Fatal("language flag is not registered")
	}
	if got := languageFlag.Names(); !slices.Equal(got, []string{"language", "l", "languages"}) {
		t.Fatalf("language flag names = %v, want language, l, languages", got)
	}
	if languageFlag.Usage == "" {
		t.Fatal("language flag help is empty")
	}
}

func TestGenerateCompatibilityFixtureUsesPackageJSONHistoricalDefaults(t *testing.T) {
	t.Helper()

	projectDir := t.TempDir()
	rootDir := repoRoot(t)
	goMod := []byte("module example.com/compatibility\n\ngo 1.25.0\n\nrequire github.com/aperturerobotics/common v0.0.0\n\nreplace github.com/aperturerobotics/common => " + filepath.ToSlash(rootDir) + "\n")
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), goMod, 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "go.sum"), []byte("github.com/aperturerobotics/protobuf-go-lite v0.16.0 h1:McGR0jrc15ZkH8HUpAARDOtazjwqr+uYXVHrrR59K28=\ngithub.com/aperturerobotics/protobuf-go-lite v0.16.0/go.mod h1:3Ay/E7iaw2KWLirK3+dDdNJZHK0hu8Y1/kKeYeUa+8s=\n"), 0o644); err != nil {
		t.Fatalf("write go.sum: %v", err)
	}
	fixture, err := os.ReadFile(filepath.Join(rootDir, "example", "compatibility.proto"))
	if err != nil {
		t.Fatalf("read compatibility fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compatibility.proto"), fixture, 0o644); err != nil {
		t.Fatalf("write compatibility fixture: %v", err)
	}
	options, err := os.ReadFile(filepath.Join(rootDir, "example", "compatibility_options.proto"))
	if err != nil {
		t.Fatalf("read compatibility options: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "compatibility_options.proto"), options, 0o644); err != nil {
		t.Fatalf("write compatibility options: %v", err)
	}
	wktPath := filepath.Join(projectDir, "vendor", "github.com", "aperturerobotics", "protobuf", "src", "google", "protobuf", "timestamp.proto")
	if err := os.MkdirAll(filepath.Dir(wktPath), 0o755); err != nil {
		t.Fatalf("create well-known type directory: %v", err)
	}
	for _, name := range []string{"timestamp.proto", "descriptor.proto"} {
		wkt, err := os.ReadFile(filepath.Join(rootDir, "vendor", "github.com", "aperturerobotics", "protobuf", "src", "google", "protobuf", name))
		if err != nil {
			t.Fatalf("read well-known fixture %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(filepath.Dir(wktPath), name), wkt, 0o644); err != nil {
			t.Fatalf("write well-known fixture %s: %v", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(projectDir, "package.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}
	if err := os.Symlink(filepath.Join(rootDir, "node_modules"), filepath.Join(projectDir, "node_modules")); err != nil {
		t.Fatalf("link node_modules: %v", err)
	}
	runTestCommand(t, projectDir, "git", "init")
	runTestCommand(t, projectDir, "git", "add", "compatibility.proto", "compatibility_options.proto")

	cfg := protogen.NewConfig()
	cfg.ProjectDir = projectDir
	cfg.Force = true
	runTestCommand(t, projectDir, "go", "mod", "download")
	if err := ensureDeps(cfg.ProjectDir, cfg.ToolsDir, false); err != nil {
		t.Fatalf("ensure deps: %v", err)
	}
	// Test boundary: default discovery and output accounting are asserted here;
	// descriptor option preservation and schema-evolution unknown-field behavior
	// are separate compatibility contracts.
	gen, err := protogen.NewGenerator(cfg)
	if err != nil {
		t.Fatalf("new generator: %v", err)
	}
	if err := gen.Generate(t.Context()); err != nil {
		t.Fatalf("generate: %v", err)
	}

	messageOutputs := map[string][]string{
		"compatibility":         {".proto", ".pb.go", ".pb.ts", ".pb.cc", ".pb.h", ".pb.rs"},
		"compatibility_options": {".proto", ".pb.go", ".pb.ts", ".pb.cc", ".pb.h"},
	}
	for base, suffixes := range messageOutputs {
		for _, suffix := range suffixes {
			if _, err := os.Stat(filepath.Join(projectDir, base+suffix)); err != nil {
				t.Fatalf("missing historical-default output %s%s: %v", base, suffix, err)
			}
		}
	}
	for _, base := range []string{"compatibility", "compatibility_options"} {
		for _, suffix := range []string{"_srpc.pb.go", "_srpc.pb.ts", "_srpc.pb.cpp", "_srpc.pb.hpp", "_srpc.pb.rs"} {
			if _, err := os.Stat(filepath.Join(projectDir, base+suffix)); err != nil {
				t.Fatalf("missing historical-default service output %s%s: %v", base, suffix, err)
			}
		}
	}
	for _, base := range []string{"compatibility", "compatibility_options"} {
		if _, err := os.Stat(filepath.Join(projectDir, base+"_pb2.py")); !os.IsNotExist(err) {
			t.Fatalf("historical default unexpectedly emitted Python for %s: %v", base, err)
		}
	}
}

const fakeStarpcPythonSource = `package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	req, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var names []string
	pos := 0
	for pos < len(req) {
		tag, n := binary.Uvarint(req[pos:])
		if n <= 0 {
			break
		}
		pos += n
		field := int(tag >> 3)
		wire := int(tag & 7)
		switch wire {
		case 0:
			if _, n := binary.Uvarint(req[pos:]); n <= 0 {
				break
			}
			pos += n
		case 1:
			pos += 8
		case 2:
			l, n := binary.Uvarint(req[pos:])
			if n <= 0 {
				break
			}
			pos += n
			if field == 1 {
				names = append(names, string(req[pos:pos+int(l)]))
			}
			pos += int(l)
		case 5:
			pos += 4
		}
	}
	var resp []byte
	for _, name := range names {
		base := strings.TrimSuffix(name, ".proto")
		var file []byte
		file = appendString(file, 1, base+"_srpc.py")
		file = appendString(file, 15, "generated stub for "+base+"\n")
		resp = appendBytes(resp, 15, file)
		file = file[:0]
		file = appendString(file, 1, base+"_srpc.pyi")
		file = appendString(file, 15, "generated stub types for "+base+"\n")
		resp = appendBytes(resp, 15, file)
	}
	os.Stdout.Write(resp)
}

func appendString(dst []byte, field int, value string) []byte {
	return appendBytes(dst, field, []byte(value))
}

func appendBytes(dst []byte, field int, payload []byte) []byte {
	dst = binary.AppendUvarint(dst, uint64((field<<3)|2))
	dst = binary.AppendUvarint(dst, uint64(len(payload)))
	return append(dst, payload...)
}
`

func TestGenerateStarpcPythonServiceOutputsAndStaleRemoval(t *testing.T) {
	t.Helper()

	projectDir := t.TempDir()
	fakeSrc := t.TempDir()
	if err := os.WriteFile(filepath.Join(fakeSrc, "main.go"), []byte(fakeStarpcPythonSource), 0o644); err != nil {
		t.Fatalf("write fake plugin source: %v", err)
	}
	toolsBin := filepath.Join(projectDir, ".tools", "bin")
	if err := os.MkdirAll(toolsBin, 0o755); err != nil {
		t.Fatalf("create tools bin: %v", err)
	}
	rootDir := repoRoot(t)
	runTestCommand(t, rootDir, "go", "build", "-o", filepath.Join(toolsBin, "protoc-gen-starpc-python"), filepath.Join(fakeSrc, "main.go"))

	goMod := []byte("module example.com/scratch\n\ngo 1.25.0\n")
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), goMod, 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "go.sum"), []byte("github.com/aperturerobotics/protobuf-go-lite v0.16.0 h1:McGR0jrc15ZkH8HUpAARDOtazjwqr+uYXVHrrR59K28=\ngithub.com/aperturerobotics/protobuf-go-lite v0.16.0/go.mod h1:3Ay/E7iaw2KWLirK3+dDdNJZHK0hu8Y1/kKeYeUa+8s=\n"), 0o644); err != nil {
		t.Fatalf("write go.sum: %v", err)
	}
	protoFile := []byte(`syntax = "proto3";
package scratch;

// Scratch contains the value used by this generation test.
message Scratch {
  // Value contains the generated test value.
  string value = 1;
}

// ScratchService exposes Scratch requests for this generation test.
service ScratchService {
  // Get returns the requested Scratch value.
  rpc Get(Scratch) returns (Scratch);
}
`)
	if err := os.WriteFile(filepath.Join(projectDir, "scratch.proto"), protoFile, 0o644); err != nil {
		t.Fatalf("write scratch.proto: %v", err)
	}
	runTestCommand(t, projectDir, "git", "init")
	runTestCommand(t, projectDir, "git", "add", "scratch.proto")

	cfg := protogen.NewConfig()
	cfg.ProjectDir = projectDir
	cfg.Languages = []string{"python"}
	cfg.RPCLibraries = []string{"starpc-python"}

	gen, err := protogen.NewGenerator(cfg)
	if err != nil {
		t.Fatalf("new generator: %v", err)
	}
	if err := gen.Generate(t.Context()); err != nil {
		t.Fatalf("generate: %v", err)
	}
	for _, name := range []string{"scratch_pb2.py", "scratch_pb2.pyi", "scratch_srpc.py", "scratch_srpc.pyi"} {
		if _, err := os.Stat(filepath.Join(projectDir, name)); err != nil {
			t.Fatalf("missing explicit service output %s: %v", name, err)
		}
	}
	stubBytes, err := os.ReadFile(filepath.Join(projectDir, "scratch_srpc.py"))
	if err != nil {
		t.Fatalf("read service stub: %v", err)
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
	if got, err := os.ReadFile(filepath.Join(projectDir, "scratch_srpc.py")); err != nil || !bytes.Equal(got, stubBytes) {
		t.Fatalf("service stub second-run changed: %v", err)
	}

	cfg.RPCLibraries = []string{"none"}
	gen, err = protogen.NewGenerator(cfg)
	if err != nil {
		t.Fatalf("new message-only generator: %v", err)
	}
	if err := gen.Generate(t.Context()); err != nil {
		t.Fatalf("message-only generate: %v", err)
	}
	for _, name := range []string{"scratch_srpc.py", "scratch_srpc.pyi"} {
		if _, err := os.Stat(filepath.Join(projectDir, name)); !os.IsNotExist(err) {
			t.Fatalf("expected stale service output removal for %s: %v", name, err)
		}
	}
	for _, name := range []string{"scratch_pb2.py", "scratch_pb2.pyi"} {
		if _, err := os.Stat(filepath.Join(projectDir, name)); err != nil {
			t.Fatalf("message output %s removed unexpectedly: %v", name, err)
		}
	}
}

func TestGenerateGoOnly(t *testing.T) {
	t.Helper()

	projectDir := t.TempDir()
	rootDir := repoRoot(t)

	goMod := []byte("module example.com/scratch\n\ngo 1.25.0\n\nrequire github.com/aperturerobotics/common v0.0.0\n\nreplace github.com/aperturerobotics/common => " + filepath.ToSlash(rootDir) + "\n")
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), goMod, 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "go.sum"), []byte("github.com/aperturerobotics/protobuf-go-lite v0.16.0 h1:McGR0jrc15ZkH8HUpAARDOtazjwqr+uYXVHrrR59K28=\ngithub.com/aperturerobotics/protobuf-go-lite v0.16.0/go.mod h1:3Ay/E7iaw2KWLirK3+dDdNJZHK0hu8Y1/kKeYeUa+8s=\n"), 0o644); err != nil {
		t.Fatalf("write go.sum: %v", err)
	}

	protoFile := []byte(`syntax = "proto3";
package scratch;

option go_package = "example.com/scratch";

// Scratch contains the value used by this generation test.
message Scratch {
  // Value contains the generated test value.
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

	runTestCommand(t, projectDir, "go", "mod", "download")
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
	if err := os.WriteFile(filepath.Join(projectDir, "go.sum"), []byte("github.com/aperturerobotics/protobuf-go-lite v0.16.0 h1:McGR0jrc15ZkH8HUpAARDOtazjwqr+uYXVHrrR59K28=\ngithub.com/aperturerobotics/protobuf-go-lite v0.16.0/go.mod h1:3Ay/E7iaw2KWLirK3+dDdNJZHK0hu8Y1/kKeYeUa+8s=\n"), 0o644); err != nil {
		t.Fatalf("write go.sum: %v", err)
	}

	protoFile := []byte(`syntax = "proto3";
package scratch;

option go_package = "example.com/scratch";

// Scratch contains the value used by this generation test.
message Scratch {
  // Value contains the generated test value.
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

	runTestCommand(t, projectDir, "go", "mod", "download")
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
	if err := os.WriteFile(filepath.Join(projectDir, "go.sum"), []byte("github.com/aperturerobotics/protobuf-go-lite v0.16.0 h1:McGR0jrc15ZkH8HUpAARDOtazjwqr+uYXVHrrR59K28=\ngithub.com/aperturerobotics/protobuf-go-lite v0.16.0/go.mod h1:3Ay/E7iaw2KWLirK3+dDdNJZHK0hu8Y1/kKeYeUa+8s=\n"), 0o644); err != nil {
		t.Fatalf("write go.sum: %v", err)
	}
	protoFile := []byte(`syntax = "proto3";
package scratch;

// Scratch contains the value used by this generation test.
message Scratch {
  // Value contains the generated test value.
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
	pythonStubPath := filepath.Join(projectDir, "scratch_pb2.pyi")
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
	pythonStub, err := os.ReadFile(pythonStubPath)
	if err != nil {
		t.Fatalf("read Python stub output: %v", err)
	}
	if !bytes.Contains(pythonStub, []byte("Scratch")) {
		t.Fatal("Python stub output does not contain Scratch")
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
	if got, err := os.ReadFile(pythonStubPath); err != nil || !bytes.Equal(got, pythonStub) {
		t.Fatalf("Python stub second-run output changed: %v", err)
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
	if _, err := os.Stat(pythonStubPath); !os.IsNotExist(err) {
		t.Fatalf("expected stale Python stub removal, got %v", err)
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

func TestGeneratePythonRewritesCanonicalLocalImports(t *testing.T) {
	t.Helper()
	projectDir := t.TempDir()
	rootDir := repoRoot(t)
	for rel, body := range map[string]string{
		"app/app.proto": `syntax = "proto3";
package app;
import "github.com/example/project/dep/dep.proto";
import "google/protobuf/timestamp.proto";

// App contains a dependency and its observation time.
message App {
  // Dependency contains the imported dependency value.
  dep.Dependency dependency = 1;

  // ObservedAt records when the dependency was observed.
  google.protobuf.Timestamp observed_at = 2;
}
`,
		"dep/dep.proto": `syntax = "proto3";
package dep;

// Dependency contains the imported dependency value.
message Dependency {
  // Value contains the dependency value.
  string value = 1;
}
`,
	} {
		path := filepath.Join(projectDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	wktDir := filepath.Join(projectDir, "vendor", "github.com", "aperturerobotics", "protobuf", "src", "google", "protobuf")
	if err := os.MkdirAll(wktDir, 0o755); err != nil {
		t.Fatal(err)
	}
	wkt, err := os.ReadFile(filepath.Join(rootDir, "vendor", "github.com", "aperturerobotics", "protobuf", "src", "google", "protobuf", "timestamp.proto"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wktDir, "timestamp.proto"), wkt, 0o644); err != nil {
		t.Fatal(err)
	}
	goMod := []byte("module github.com/example/project\n\ngo 1.25.0\n")
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), goMod, 0o644); err != nil {
		t.Fatal(err)
	}
	runTestCommand(t, projectDir, "git", "init")
	runTestCommand(t, projectDir, "git", "add", "app/app.proto", "dep/dep.proto")
	cfg := protogen.NewConfig()
	cfg.ProjectDir = projectDir
	cfg.Targets = []string{"./app/*.proto", "./dep/*.proto"}
	cfg.Languages = []string{"python"}
	cfg.RPCLibraries = []string{"none"}
	cfg.Force = true
	gen, err := protogen.NewGenerator(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"app/app_pb2.py", "app/app_pb2.pyi", "dep/dep_pb2.py", "dep/dep_pb2.pyi"} {
		if _, err := os.Stat(filepath.Join(projectDir, rel)); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
	for _, rel := range []string{"app/app_pb2.py", "app/app_pb2.pyi"} {
		data, err := os.ReadFile(filepath.Join(projectDir, rel))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if !strings.Contains(text, "from dep import dep_pb2") {
			t.Fatalf("%s lacks rewritten local import: %s", rel, text)
		}
		if !strings.Contains(text, "google.protobuf") {
			t.Fatalf("%s rewrote WKT import", rel)
		}
	}
	cmd := exec.Command("uv", "run", "--directory", filepath.Join(rootDir, "tests", "python"), "python", "-c", "import sys; sys.path.insert(0, sys.argv[1]); import app.app_pb2", projectDir)
	cmd.Env = append(os.Environ(), "UV_PROJECT_ENVIRONMENT="+filepath.Join(t.TempDir(), ".venv"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated Python import: %v\\n%s", err, out)
	}
}

func TestGenerateExplicitUntrackedAndMissingTargets(t *testing.T) {
	t.Helper()
	projectDir := t.TempDir()
	rootDir := repoRoot(t)
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module example.com/targettest\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "untracked.proto"), []byte(`syntax = "proto3";
package targettest;
message Target {}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("go", "run", "./cmd/aptre", "generate", "--deps=false", "--project-dir", projectDir, "--targets", "untracked.proto", "--language", "csharp")
	cmd.Dir = rootDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("explicit untracked target: %v\n%s", err, output)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "Untracked.cs")); err != nil {
		t.Fatalf("untracked target output: %v", err)
	}

	cmd = exec.Command("go", "run", "./cmd/aptre", "generate", "--deps=false", "--project-dir", projectDir, "--targets", "missing.proto")
	cmd.Dir = rootDir
	output, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "matched no proto sources") {
		t.Fatalf("missing target error = %v\n%s", err, output)
	}

	if err := os.WriteFile(filepath.Join(projectDir, "strict.proto"), []byte(`syntax = "proto3";
message Strict { string key = 1; }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("go", "run", "./cmd/aptre", "generate", "--deps=false", "--check-proto-contracts", "--project-dir", projectDir, "--targets", "strict.proto")
	cmd.Dir = rootDir
	output, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "must use an expanded block") {
		t.Fatalf("strict target error = %v\n%s", err, output)
	}
}
