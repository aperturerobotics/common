package protogen

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestFindGeneratedFilesForProtoBuiltInLanguages(t *testing.T) {
	projectDir := t.TempDir()
	vendorDir := filepath.Join(projectDir, "vendor")
	modulePath := "example.com/project"
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
		vendorDir,
		modulePath,
		Languages{LanguageGo: {}, LanguageCSharp: {}, LanguagePython: {}},
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
	vendorDir := filepath.Join(projectDir, "vendor")
	if err := os.MkdirAll(vendorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "Other.cs"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := FindGeneratedFilesForProto(
		"match_state.proto",
		projectDir,
		vendorDir,
		"example.com/project",
		Languages{LanguageCSharp: {}, LanguagePython: {}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("unexpected generated files: %v", got)
	}
}
