package golangcilint

import (
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestBuildConfigPreservesPluginInputs(t *testing.T) {
	project := t.TempDir()
	destination := filepath.Join(project, ".tools", "build")
	data, err := BuildConfig(filepath.Join(project, ".custom-gcl.yml"), []byte(`
version: v2.1.6
name: project-lint
destination: ./output
plugins:
  - module: example.com/local
    import: example.com/local/analyzer
    path: ./lint/local
  - module: example.com/published
    version: v1.2.3
`), destination)
	if err != nil {
		t.Fatal(err)
	}
	var conf struct {
		Version     string
		Name        string
		Destination string
		Plugins     []map[string]string
	}
	if err := yaml.Unmarshal(data, &conf); err != nil {
		t.Fatal(err)
	}
	if conf.Version != "v2.1.6" || conf.Name != "golangci-lint" || conf.Destination != destination {
		t.Fatalf("unexpected build destination: %+v", conf)
	}
	if len(conf.Plugins) != 2 {
		t.Fatalf("plugins = %v", conf.Plugins)
	}
	local, published := conf.Plugins[0], conf.Plugins[1]
	if local["path"] != filepath.Join(project, "lint", "local") || local["import"] != "example.com/local/analyzer" || local["module"] != "example.com/local" {
		t.Fatalf("local plugin changed: %v", local)
	}
	if published["module"] != "example.com/published" || published["version"] != "v1.2.3" || published["path"] != "" {
		t.Fatalf("published plugin changed: %v", published)
	}
}

func TestBuildConfigRequiresVersion(t *testing.T) {
	if _, err := BuildConfig(".custom-gcl.yml", []byte("plugins: []"), t.TempDir()); err == nil {
		t.Fatal("accepted a configuration without a version")
	}
}
