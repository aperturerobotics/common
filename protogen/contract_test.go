package protogen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckProtoContractsRejectsHouseStyleViolations(t *testing.T) {
	tests := []struct {
		name  string
		proto string
		wants []string
	}{
		{
			name: "one line message body",
			proto: `syntax = "proto3";

// Record stores one record.
message Record { string key = 1; }
`,
			wants: []string{"must use an expanded block"},
		},
		{
			name: "missing declaration separation",
			proto: `syntax = "proto3";

// First contains the first value.
message First {
}
// Second contains the second value.
message Second {
}
`,
			wants: []string{"blank line"},
		},
		{
			name: "missing declaration comments",
			proto: `syntax = "proto3";

enum State {
  STATE_UNKNOWN = 0;
}

message Record {
  string key = 1;
}

service Records {
  rpc Read(Record) returns (Record);
}
`,
			wants: []string{
				"enum State requires a semantic comment",
				"enum value STATE_UNKNOWN requires a semantic comment",
				"message Record requires a semantic comment",
				"field key requires a semantic comment",
				"service Records requires a semantic comment",
				"RPC Read requires a semantic comment",
			},
		},
		{
			name: "enum zero is not unknown",
			proto: `syntax = "proto3";

// State describes a record state.
enum State {
  // STATE_UNSPECIFIED is the default record state.
  STATE_UNSPECIFIED = 0;
}
`,
			wants: []string{"UNKNOWN = 0"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := writeContractProto(t, test.proto)
			err := CheckProtoContracts(filepath.Dir(path), []string{filepath.Base(path)})
			if err == nil {
				t.Fatal("CheckProtoContracts() unexpectedly succeeded")
			}
			for _, want := range test.wants {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("CheckProtoContracts() error = %v, want %q", err, want)
				}
			}
		})
	}
}

func TestCheckProtoContractsAcceptsDocumentedExpandedSchema(t *testing.T) {
	path := writeContractProto(t, `syntax = "proto3";

// State describes the lifecycle state of a record.
enum State {
  // STATE_UNKNOWN is the default state when no lifecycle state is known.
  STATE_UNKNOWN = 0;

  // READY indicates that the record accepts reads.
  READY = 1;
}

// Record contains the data returned by Records.
message Record {
  // RecordKey identifies the record returned by Records.
  string record_key = 1;
}

// Records exposes record reads.
service Records {
  // Read returns the record identified by its request.
  rpc Read(Record) returns (Record);
}
`)

	if err := CheckProtoContracts(filepath.Dir(path), []string{filepath.Base(path)}); err != nil {
		t.Fatalf("CheckProtoContracts(): %v", err)
	}
}

func TestGeneratorLeavesLegacyContractsCompatible(t *testing.T) {
	dir := t.TempDir()
	runContractCommand(t, dir, "git", "init")
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/contracts\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "legacy.proto"), []byte("syntax = \"proto3\";\nmessage Legacy { string key = 1; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runContractCommand(t, dir, "git", "add", "legacy.proto")

	gen := &Generator{
		Config:     &Config{Targets: []string{"*.proto"}},
		Cache:      NewCache(),
		Plugins:    &Plugins{Languages: Languages{}, RPCLibraries: RPCLibraries{}},
		ProjectDir: dir,
		ModuleDir:  dir,
		ModulePath: "example.com/contracts",
		VendorDir:  filepath.Join(dir, "vendor"),
		OutDir:     filepath.Join(dir, "vendor"),
	}
	if err := gen.Generate(t.Context()); err == nil || !strings.Contains(err.Error(), "Missing output directives") {
		t.Fatalf("Generate() applied the strict legacy check: %v", err)
	}
}

func TestCheckProtoContractsHandlesMultilineDeclarationsAndLiteralBraces(t *testing.T) {
	path := writeContractProto(t, `syntax = "proto3";

// State describes the state of a record.
enum State
{
  // STATE_UNKNOWN is the default record state.
  STATE_UNKNOWN = 0;
}

// Record contains the service request.
message Record
{
  // Key identifies the requested record.
  string key = 1 [json_name = "key{}/*not a comment*/"];
}

// Records exposes record operations.
service Records
{
  // Read returns the requested record.
  rpc Read(Record) returns (Record) {
    option deprecated = false; // } ignored
  }
}
`)
	if err := CheckProtoContracts(filepath.Dir(path), []string{filepath.Base(path)}); err != nil {
		t.Fatalf("CheckProtoContracts(): %v", err)
	}
}

func TestCheckProtoContractsSkipsVendoredAndGeneratedSources(t *testing.T) {
	dir := t.TempDir()
	paths := []string{
		filepath.Join(dir, "vendor", "example.com", "upstream.proto"),
		filepath.Join(dir, "api", "generated.pb.proto"),
	}
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("syntax = \"proto3\";\nmessage Upstream { string key = 1; }\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := CheckProtoContracts(dir, []string{filepath.Join("vendor", "example.com", "upstream.proto"), filepath.Join("api", "generated.pb.proto")}); err != nil {
		t.Fatalf("CheckProtoContracts() validated exempt source: %v", err)
	}
}

func TestDiscoverProtoFilesReportsTargetWithoutSources(t *testing.T) {
	dir := t.TempDir()
	runContractCommand(t, dir, "git", "init")
	if err := os.WriteFile(filepath.Join(dir, "present.proto"), []byte("syntax = \"proto3\";\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runContractCommand(t, dir, "git", "add", "present.proto")

	_, unmatched, err := discoverProtoFiles(dir, []string{"missing/*.proto"}, nil)
	if err != nil {
		t.Fatalf("discoverProtoFiles(): %v", err)
	}
	if len(unmatched) != 1 || unmatched[0] != "missing/*.proto" {
		t.Fatalf("unmatched targets = %v, want missing target", unmatched)
	}
}

func TestDiscoverProtoFilesAdmitsExplicitUntrackedSource(t *testing.T) {
	dir := t.TempDir()
	runContractCommand(t, dir, "git", "init")
	if err := os.WriteFile(filepath.Join(dir, "untracked.proto"), []byte("syntax = \"proto3\";\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, unmatched, err := discoverProtoFiles(dir, []string{"untracked.proto"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(unmatched) != 0 || len(files) != 1 || files[0] != "untracked.proto" {
		t.Fatalf("files=%v unmatched=%v, want explicit untracked source", files, unmatched)
	}
}

func TestDiscoverProtoFilesRejectsEscapingTarget(t *testing.T) {
	dir := t.TempDir()
	_, _, err := discoverProtoFiles(dir, []string{"../outside.proto"}, nil)
	if err == nil || !strings.Contains(err.Error(), "escapes project directory") {
		t.Fatalf("discoverProtoFiles() error = %v, want escape diagnostic", err)
	}
}

func TestGeneratorRejectsInvalidFirstPartyContract(t *testing.T) {
	dir := t.TempDir()
	runContractCommand(t, dir, "git", "init")
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/contracts\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "contract.proto"), []byte("syntax = \"proto3\";\nmessage Contract { string key = 1; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runContractCommand(t, dir, "git", "add", "contract.proto")

	gen := &Generator{
		Config:     &Config{Targets: []string{"*.proto"}, CheckProtoContracts: true},
		ProjectDir: dir,
		ModulePath: "example.com/contracts",
		VendorDir:  filepath.Join(dir, "vendor"),
	}
	if err := gen.Generate(t.Context()); err == nil || !strings.Contains(err.Error(), "must use an expanded block") {
		t.Fatalf("Generate() error = %v, want proto contract diagnostic", err)
	}
}

func TestGeneratorKeepsDefaultEmptyProjectBehavior(t *testing.T) {
	dir := t.TempDir()
	runContractCommand(t, dir, "git", "init")
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/contracts\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gen := &Generator{
		Config:     NewConfig(),
		ProjectDir: dir,
		ModulePath: "example.com/contracts",
		VendorDir:  filepath.Join(dir, "vendor"),
	}
	if err := gen.Generate(t.Context()); err != nil {
		t.Fatalf("Generate() default empty project: %v", err)
	}
}

func TestGeneratorRejectsExplicitUnmatchedTarget(t *testing.T) {
	dir := t.TempDir()
	runContractCommand(t, dir, "git", "init")
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/contracts\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	gen := &Generator{
		Config: &Config{
			Targets:         []string{"missing/*.proto"},
			TargetsExplicit: true,
		},
		ProjectDir: dir,
		ModulePath: "example.com/contracts",
		VendorDir:  filepath.Join(dir, "vendor"),
	}
	if err := gen.Generate(t.Context()); err == nil || !strings.Contains(err.Error(), "matched no proto sources") {
		t.Fatalf("Generate() error = %v, want unmatched-target diagnostic", err)
	}
}

func writeContractProto(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "contract.proto")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func runContractCommand(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, output)
	}
}
