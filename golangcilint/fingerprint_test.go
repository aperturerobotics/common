package golangcilint

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestFingerprint checks that changed configuration, source, and assets rebuild.
func TestFingerprint(t *testing.T) {
	// Build a local plugin whose path requires YAML quoting.
	dir := t.TempDir()
	plugin := filepath.Join(dir, "my plugin")
	if err := os.Mkdir(plugin, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, data string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(plugin, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("plugin.go", "package plugin")
	config := []byte("version: v2.13.2\nplugins:\n  - module: example.com/plugin\n    path: 'my plugin'\n")
	path := filepath.Join(dir, ".custom-gcl.yml")
	fingerprint := func(data []byte) string {
		t.Helper()
		sum, err := Fingerprint(path, data)
		if err != nil {
			t.Fatal(err)
		}
		return sum
	}
	original := fingerprint(config)
	if got := fingerprint(config); got != original {
		t.Fatal("unchanged inputs produced different fingerprints")
	}

	// Config-only plugin changes must invalidate a previously built binary.
	changed := append(slices.Clone(config), []byte("  - module: example.com/other\n    version: v1.0.0\n")...)
	if fingerprint(changed) == original {
		t.Fatal("changed plugin list reused the original fingerprint")
	}
	for _, name := range []string{"plugin.go", "go.mod", "go.sum", "embedded.txt"} {
		before := fingerprint(config)
		write(name, "changed "+name)
		if fingerprint(config) == before {
			t.Fatalf("changing %s did not invalidate the fingerprint", name)
		}
	}

	// Deleting an input also changes the fingerprint.
	before := fingerprint(config)
	if err := os.Remove(filepath.Join(plugin, "embedded.txt")); err != nil {
		t.Fatal(err)
	}
	if fingerprint(config) == before {
		t.Fatal("removing an input did not invalidate the fingerprint")
	}
}

// TestFingerprintErrors rejects malformed YAML and missing local plugins.
func TestFingerprintErrors(t *testing.T) {
	for _, config := range []string{"plugins: [", "plugins:\n - path: missing-plugin\n"} {
		if _, err := Fingerprint(filepath.Join(t.TempDir(), ".custom-gcl.yml"), []byte(config)); err == nil {
			t.Fatalf("accepted invalid inputs %q", config)
		}
	}
}
