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
