package protogen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProcessTsFileRewritesCrossBoundaryImports(t *testing.T) {
	t.Helper()

	projectDir := t.TempDir()
	filePath := filepath.Join(projectDir, "bldr", "plugin", "plugin.pb.ts")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir ts dir: %v", err)
	}

	content := `// @generated from file github.com/s4wave/spacewave/bldr/plugin/plugin.proto
import { VolumeInfo } from "../../db/volume/volume.pb.js"
`
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write ts file: %v", err)
	}

	pp := NewPostProcessor(
		projectDir,
		filepath.Join(projectDir, "vendor"),
		"github.com/s4wave/spacewave",
		[]string{"bldr", "db", "net"},
		false,
	)
	if err := pp.ProcessTsFile(filePath); err != nil {
		t.Fatalf("process ts file: %v", err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read ts file: %v", err)
	}
	got := string(data)
	want := `// @generated from file github.com/s4wave/spacewave/bldr/plugin/plugin.proto
import { VolumeInfo } from "@go/github.com/s4wave/spacewave/db/volume/volume.pb.js"`
	if strings.TrimSpace(got) != want {
		t.Fatalf("expected rewritten import:\n%s\ngot:\n%s", want, got)
	}
}

func TestProcessTsFileKeepsSameBoundaryImportsRelative(t *testing.T) {
	t.Helper()

	projectDir := t.TempDir()
	filePath := filepath.Join(projectDir, "db", "bucket", "bucket.pb.ts")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir ts dir: %v", err)
	}

	content := `// @generated from file github.com/s4wave/spacewave/db/bucket/bucket.proto
import { BlockRef } from "../block/block.pb.js"
`
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write ts file: %v", err)
	}

	pp := NewPostProcessor(
		projectDir,
		filepath.Join(projectDir, "vendor"),
		"github.com/s4wave/spacewave",
		[]string{"bldr", "db", "net"},
		false,
	)
	if err := pp.ProcessTsFile(filePath); err != nil {
		t.Fatalf("process ts file: %v", err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read ts file: %v", err)
	}
	got := string(data)
	if strings.TrimSpace(got) != strings.TrimSpace(content) {
		t.Fatalf("expected relative import to remain unchanged:\n%s\ngot:\n%s", content, got)
	}
}

func TestProcessPythonFileRewritesLocalImportsAndPreservesExternal(t *testing.T) {
	projectDir := t.TempDir()
	file := filepath.Join(projectDir, "app_pb2.py")
	content := "from github.com.example.my_project.dep import dep_pb2 as dep\nfrom google.protobuf import timestamp_pb2 as ts\nfrom external.pkg import other\nDESCRIPTOR = 'github.com.example.my_project.app/app.proto'\n"
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	pp := NewPostProcessor(projectDir, "", "github.com/example/my-project", nil, false)
	if err := pp.ProcessPythonFile(file); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(file)
	text := string(got)
	if !strings.Contains(text, "from dep import dep_pb2 as dep") || !strings.Contains(text, "from google.protobuf") || !strings.Contains(text, "from external.pkg") || !strings.Contains(text, "DESCRIPTOR =") {
		t.Fatalf("unexpected rewrite: %s", text)
	}
	before := text
	if err := pp.ProcessPythonFile(file); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(file)
	if string(after) != before {
		t.Fatal("second pass changed output")
	}
}

func TestProcessPythonFileRewritesPyi(t *testing.T) {
	projectDir := t.TempDir()
	file := filepath.Join(projectDir, "app_pb2.pyi")
	os.WriteFile(file, []byte("from github.com.example.mod.dep import dep_pb2 as _dep_pb2\n"), 0o644)
	pp := NewPostProcessor(projectDir, "", "github.com/example/mod", nil, false)
	if err := pp.ProcessPythonFile(file); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(file)
	if string(got) != "from dep import dep_pb2 as _dep_pb2\n" {
		t.Fatalf("got %q", got)
	}
}
