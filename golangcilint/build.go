package golangcilint

import (
	"path/filepath"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// BuildConfig prepares an isolated custom build without changing project files.
// Local plugin paths remain relative to the original configuration directory;
// other builder settings pass through unchanged.
func BuildConfig(configPath string, config []byte, destination string) ([]byte, error) {
	var conf struct {
		Version     string `yaml:"version"`
		Name        string `yaml:"name"`
		Destination string `yaml:"destination"`
		Plugins     []struct {
			Path   string         `yaml:"path,omitempty"`
			Fields map[string]any `yaml:",inline"`
		} `yaml:"plugins"`
		Fields map[string]any `yaml:",inline"`
	}
	if err := yaml.Unmarshal(config, &conf); err != nil {
		return nil, errors.Wrap(err, "parse custom golangci-lint configuration")
	}
	if conf.Version == "" {
		return nil, errors.Errorf("missing version in %s", configPath)
	}
	conf.Name = "golangci-lint"
	conf.Destination = destination
	for i := range conf.Plugins {
		plugin := &conf.Plugins[i]
		if plugin.Path != "" && !filepath.IsAbs(plugin.Path) {
			plugin.Path = filepath.Join(filepath.Dir(configPath), plugin.Path)
		}
	}
	return yaml.Marshal(&conf)
}
