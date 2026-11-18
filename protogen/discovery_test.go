package protogen

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestFindGeneratedFilesForProtoBuiltInLanguages(t *testing.T) {
	projectDir := t.TempDir()
	protoDir := "api/v1"

	generated := []string{
		filepath.Join(projectDir, protoDir, "match_state.pb.go"),
		filepath.Join(projectDir, protoDir, "match_state_pb2.py"),
		filepath.Join(projectDir, "MatchState.cs"),
	}
	for _, path := range generated {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := FindGeneratedFilesForProto(
		filepath.Join(protoDir, "match_state.proto"),
		projectDir,
		Languages{LanguageGo: {}, LanguageCSharp: {}, LanguagePython: {}},
		RPCLibraries{},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"MatchState.cs",
		filepath.Join(protoDir, "match_state.pb.go"),
		filepath.Join(protoDir, "match_state_pb2.py"),
	}
	if !slices.Equal(got, want) {
		t.Fatalf("generated files: want %v, got %v", want, got)
	}
}

func TestFindGeneratedFilesForProtoDoesNotClaimUnrelatedCSharp(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "Other.cs"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := FindGeneratedFilesForProto(
		"match_state.proto",
		projectDir,
		Languages{LanguageCSharp: {}, LanguagePython: {}},
		RPCLibraries{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("unexpected generated files: %v", got)
	}
}

func TestFindGeneratedFilesForProtoStarpcPythonClaimsServiceOutputs(t *testing.T) {
	projectDir := t.TempDir()
	for _, name := range []string{"match_state_pb2.py", "match_state_pb2.pyi", "match_state_srpc.py", "match_state_srpc.pyi"} {
		if err := os.WriteFile(filepath.Join(projectDir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := FindGeneratedFilesForProto(
		"match_state.proto", projectDir,
		Languages{LanguagePython: {}}, RPCLibraries{RPCLibraryStarpcPython: {}},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"match_state_pb2.py", "match_state_pb2.pyi", "match_state_srpc.py", "match_state_srpc.pyi"}
	if !slices.Equal(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestFindGeneratedFilesForProtoStarpcPythonUnselected(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "match_state_srpc.py"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := FindGeneratedFilesForProto(
		"match_state.proto", projectDir,
		Languages{LanguagePython: {}}, RPCLibraries{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("unexpected outputs without starpc-python: %v", got)
	}
}
