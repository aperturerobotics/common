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
	// All documented language aliases resolve through the same flag.
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
	// Keep schemas, generated output and tool installation inside an isolated project.
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

	// Supply module identity and dependencies through the normal project files.
	goMod := []byte("module example.com/scratch\n\ngo 1.25.0\n")
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), goMod, 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "go.sum"), []byte("github.com/aperturerobotics/protobuf-go-lite v0.16.0 h1:McGR0jrc15ZkH8HUpAARDOtazjwqr+uYXVHrrR59K28=\ngithub.com/aperturerobotics/protobuf-go-lite v0.16.0/go.mod h1:3Ay/E7iaw2KWLirK3+dDdNJZHK0hu8Y1/kKeYeUa+8s=\n"), 0o644); err != nil {
		t.Fatalf("write go.sum: %v", err)
	}

	// Track a real schema so discovery follows the production Git path.
	protoFile := []byte(`syntax = "proto3";
package scratch;

message Scratch {
  string value = 1;
}

service ScratchService {
  rpc Get(Scratch) returns (Scratch);
}
`)
	if err := os.WriteFile(filepath.Join(projectDir, "scratch.proto"), protoFile, 0o644); err != nil {
		t.Fatalf("write scratch.proto: %v", err)
	}
	runTestCommand(t, projectDir, "git", "init")
	runTestCommand(t, projectDir, "git", "add", "scratch.proto")

	// Select generation through the public project configuration.
	cfg := protogen.NewConfig()
	cfg.ProjectDir = projectDir
	cfg.Languages = []string{"python"}
	cfg.RPCLibraries = []string{"starpc-python"}

	// Execute the embedded protoc pipeline with the selected generators.
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

	// A second run must reuse unchanged output from the generation cache.
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

	// Disabling services removes their obsolete files while retaining messages.
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
	// Keep schemas, generated output and tool installation inside an isolated project.
	projectDir := t.TempDir()
	rootDir := repoRoot(t)

	// Supply module identity and dependencies through the normal project files.
	goMod := []byte("module example.com/scratch\n\ngo 1.25.0\n\nrequire github.com/aperturerobotics/common v0.0.0\n\nreplace github.com/aperturerobotics/common => " + filepath.ToSlash(rootDir) + "\n")
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), goMod, 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "go.sum"), []byte("github.com/aperturerobotics/protobuf-go-lite v0.16.0 h1:McGR0jrc15ZkH8HUpAARDOtazjwqr+uYXVHrrR59K28=\ngithub.com/aperturerobotics/protobuf-go-lite v0.16.0/go.mod h1:3Ay/E7iaw2KWLirK3+dDdNJZHK0hu8Y1/kKeYeUa+8s=\n"), 0o644); err != nil {
		t.Fatalf("write go.sum: %v", err)
	}

	// Track a real schema so discovery follows the production Git path.
	protoFile := []byte(`syntax = "proto3";
package scratch;

option go_package = "example.com/scratch";

message Scratch {
  string value = 1;
}
`)
	if err := os.WriteFile(filepath.Join(projectDir, "scratch.proto"), protoFile, 0o644); err != nil {
		t.Fatalf("write scratch.proto: %v", err)
	}
	runTestCommand(t, projectDir, "git", "init")
	runTestCommand(t, projectDir, "git", "add", "scratch.proto")

	// Select generation through the public project configuration.
	cfg := protogen.NewConfig()
	cfg.ProjectDir = projectDir
	cfg.Force = true
	cfg.Languages = []string{"go"}

	// Prepare the normal project tools before invoking the generator.
	runTestCommand(t, projectDir, "go", "mod", "download")
	if err := ensureDeps(cfg.ProjectDir, cfg.ToolsDir, false); err != nil {
		t.Fatalf("ensure deps: %v", err)
	}

	// Execute the embedded protoc pipeline with the selected generators.
	gen, err := protogen.NewGenerator(cfg)
	if err != nil {
		t.Fatalf("new generator: %v", err)
	}
	if err := gen.Generate(t.Context()); err != nil {
		t.Fatalf("generate: %v", err)
	}

	// Inspect the actual files produced beside the schema.
	matches, err := filepath.Glob(filepath.Join(projectDir, "scratch*"))
	if err != nil {
		t.Fatalf("glob generated files: %v", err)
	}

	// The output set contains only the requested message and service languages.
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
	// Keep schemas, generated output and tool installation inside an isolated project.
	projectDir := t.TempDir()
	rootDir := repoRoot(t)

	// Supply module identity and dependencies through the normal project files.
	goMod := []byte("module example.com/scratch\n\ngo 1.25.0\n\nrequire github.com/aperturerobotics/common v0.0.0\n\nreplace github.com/aperturerobotics/common => " + filepath.ToSlash(rootDir) + "\n")
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), goMod, 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "go.sum"), []byte("github.com/aperturerobotics/protobuf-go-lite v0.16.0 h1:McGR0jrc15ZkH8HUpAARDOtazjwqr+uYXVHrrR59K28=\ngithub.com/aperturerobotics/protobuf-go-lite v0.16.0/go.mod h1:3Ay/E7iaw2KWLirK3+dDdNJZHK0hu8Y1/kKeYeUa+8s=\n"), 0o644); err != nil {
		t.Fatalf("write go.sum: %v", err)
	}

	// Track a real schema so discovery follows the production Git path.
	protoFile := []byte(`syntax = "proto3";
package scratch;

option go_package = "example.com/scratch";

message Scratch {
  string value = 1;
}
`)
	if err := os.WriteFile(filepath.Join(projectDir, "scratch.proto"), protoFile, 0o644); err != nil {
		t.Fatalf("write scratch.proto: %v", err)
	}
	runTestCommand(t, projectDir, "git", "init")
	runTestCommand(t, projectDir, "git", "add", "scratch.proto")

	// Select generation through the public project configuration.
	cfg := protogen.NewConfig()
	cfg.ProjectDir = projectDir
	cfg.Force = true
	cfg.Languages = []string{"go"}
	cfg.RPCLibraries = []string{"none"}

	// Prepare the normal project tools before invoking the generator.
	runTestCommand(t, projectDir, "go", "mod", "download")
	if err := ensureDeps(cfg.ProjectDir, cfg.ToolsDir, false); err != nil {
		t.Fatalf("ensure deps: %v", err)
	}

	// Execute the embedded protoc pipeline with the selected generators.
	gen, err := protogen.NewGenerator(cfg)
	if err != nil {
		t.Fatalf("new generator: %v", err)
	}
	if err := gen.Generate(t.Context()); err != nil {
		t.Fatalf("generate: %v", err)
	}

	// Inspect the actual files produced beside the schema.
	matches, err := filepath.Glob(filepath.Join(projectDir, "scratch*"))
	if err != nil {
		t.Fatalf("glob generated files: %v", err)
	}

	// The output set contains only the requested message and service languages.
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
	// Keep schemas, generated output and tool installation inside an isolated project.
	projectDir := t.TempDir()

	// Supply module identity and dependencies through the normal project files.
	goMod := []byte("module example.com/play-scratch\n\ngo 1.25.0\n")
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), goMod, 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "go.sum"), []byte("github.com/aperturerobotics/protobuf-go-lite v0.16.0 h1:McGR0jrc15ZkH8HUpAARDOtazjwqr+uYXVHrrR59K28=\ngithub.com/aperturerobotics/protobuf-go-lite v0.16.0/go.mod h1:3Ay/E7iaw2KWLirK3+dDdNJZHK0hu8Y1/kKeYeUa+8s=\n"), 0o644); err != nil {
		t.Fatalf("write go.sum: %v", err)
	}

	// Track a real schema so discovery follows the production Git path.
	protoFile := []byte(`syntax = "proto3";
package scratch;

message Scratch {
  string value = 1;
}
`)
	if err := os.WriteFile(filepath.Join(projectDir, "scratch.proto"), protoFile, 0o644); err != nil {
		t.Fatalf("write scratch.proto: %v", err)
	}
	runTestCommand(t, projectDir, "git", "init")
	runTestCommand(t, projectDir, "git", "add", "scratch.proto")

	// Select generation through the public project configuration.
	cfg := protogen.NewConfig()
	cfg.ProjectDir = projectDir
	cfg.Languages = []string{"csharp", "python"}

	// Execute the embedded protoc pipeline with the selected generators.
	gen, err := protogen.NewGenerator(cfg)
	if err != nil {
		t.Fatalf("new generator: %v", err)
	}
	if err := gen.Generate(t.Context()); err != nil {
		t.Fatalf("generate: %v", err)
	}

	// Both message languages must expose the schema type in their output.
	csharpPath := filepath.Join(projectDir, "Scratch.cs")
	pythonPath := filepath.Join(projectDir, "scratch_pb2.py")
	pythonStubPath := filepath.Join(projectDir, "scratch_pb2.pyi")
	csharp, err := os.ReadFile(csharpPath)
	if err != nil {
		t.Fatalf("read C# output: %v", err)
	}

	// Python generation must use its canonical module filename.
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

	// Type stubs and runtime output must both retain the generated message.
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

	// A second run must reuse unchanged output from the generation cache.
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

	// Reusing the cache leaves every generated language byte-for-byte unchanged.
	if got, err := os.ReadFile(csharpPath); err != nil || !bytes.Equal(got, csharp) {
		t.Fatalf("C# second-run output changed: %v", err)
	}
	if got, err := os.ReadFile(pythonPath); err != nil || !bytes.Equal(got, python) {
		t.Fatalf("Python second-run output changed: %v", err)
	}
	if got, err := os.ReadFile(pythonStubPath); err != nil || !bytes.Equal(got, pythonStub) {
		t.Fatalf("Python stub second-run output changed: %v", err)
	}

	// Removing a language deletes only that language's stale outputs.
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

// repoRoot locates the source checkout that contains the test fixtures.
func repoRoot(t *testing.T) string {
	t.Helper()

	// Locate fixture assets relative to this source, independent of the working directory.
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("get caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

// runTestCommand completes fixture setup or fails with the command output.
func runTestCommand(t *testing.T, dir, name string, args ...string) {
	// Attribute preparation failures to the requesting integration test.
	t.Helper()

	// Run fixture preparation in its project and retain failure diagnostics.
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, output)
	}
}

func TestGeneratePythonRewritesCanonicalLocalImports(t *testing.T) {
	// Keep schemas, generated output and tool installation inside an isolated project.
	projectDir := t.TempDir()
	rootDir := repoRoot(t)
	for rel, body := range map[string]string{
		"app/app.proto": `syntax = "proto3";
package app;
import "github.com/example/project/dep/dep.proto";
import "google/protobuf/timestamp.proto";
message App { dep.Dependency dependency = 1; google.protobuf.Timestamp observed_at = 2; }
`,
		"dep/dep.proto": `syntax = "proto3";
package dep;
message Dependency { string value = 1; }
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

	// Use the real vendored well-known schema for the external import.
	wktDir := filepath.Join(projectDir, "vendor", "github.com", "aperturerobotics", "protobuf", "src", "google", "protobuf")
	if err := os.MkdirAll(wktDir, 0o755); err != nil {
		t.Fatal(err)
	}
	wkt, err := os.ReadFile(filepath.Join(rootDir, "vendor", "github.com", "aperturerobotics", "protobuf", "src", "google", "protobuf", "timestamp.proto"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wktDir, "timestamp.proto"), wkt, 0o644); err != nil { //nolint:gosec // wktDir is inside t.TempDir.
		t.Fatal(err)
	}

	// Supply module identity and dependencies through the normal project files.
	goMod := []byte("module github.com/example/project\n\ngo 1.25.0\n")
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), goMod, 0o644); err != nil {
		t.Fatal(err)
	}
	runTestCommand(t, projectDir, "git", "init")
	runTestCommand(t, projectDir, "git", "add", "app/app.proto", "dep/dep.proto")

	// Select generation through the public project configuration.
	cfg := protogen.NewConfig()
	cfg.ProjectDir = projectDir
	cfg.Targets = []string{"./app/*.proto", "./dep/*.proto"}
	cfg.Languages = []string{"python"}
	cfg.RPCLibraries = []string{"none"}
	cfg.Force = true

	// Execute the embedded protoc pipeline with the selected generators.
	gen, err := protogen.NewGenerator(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Generate(t.Context()); err != nil {
		t.Fatal(err)
	}

	// Both schemas must produce runtime modules and type stubs.
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

	// Import the generated module with its actual Python runtime dependencies.
	cmd := exec.Command("uv", "run", "--directory", filepath.Join(rootDir, "tests", "python"), "python", "-c", "import sys; sys.path.insert(0, sys.argv[1]); import app.app_pb2", projectDir) //nolint:gosec // arguments are fixed or test-owned paths.
	cmd.Env = append(os.Environ(), "UV_PROJECT_ENVIRONMENT="+filepath.Join(t.TempDir(), ".venv"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated Python import: %v\\n%s", err, out)
	}
}

func TestGenerateCheckRequiresWholeGraphRust(t *testing.T) {
	// Keep schemas, generated output and tool installation inside an isolated project.
	projectDir := t.TempDir()
	packageJSON := []byte(`{"aptre": {"module": "example.com/scratch", "languages": ["rust", "ts"], "rust": {}}}`)
	if err := os.WriteFile(filepath.Join(projectDir, "package.json"), packageJSON, 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}

	// Read-only checking rejects mixed generation modes before writing outputs.
	app := &cli.App{Commands: []*cli.Command{generateCmd}}
	err := app.Run([]string{"aptre", "generate", "--check", "--deps=false", "-C", projectDir})
	if err == nil || !strings.Contains(err.Error(), "--check requires aptre.rust") {
		t.Fatalf("expected the check guard to reject mixed languages, got %v", err)
	}
}
