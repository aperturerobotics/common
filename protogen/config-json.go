package protogen

import (
	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
)

// FromJSON reads the typed aptre fields, allowing an absent or null section.
func (c *packageJSONAptreConfig) FromJSON(value *fastjson.Value) error {
	// A missing section leaves the caller's default configuration intact.
	if value == nil || value.Type() == fastjson.TypeNull {
		return nil
	}
	if _, err := value.Object(); err != nil {
		return errors.Wrap(err, "aptre")
	}

	// The schema identity is independent of Go module discovery.
	module, err := configString(value, "module")
	if err != nil {
		return err
	}
	c.Module = module

	// Read each language and package-boundary list without reflection.
	for _, field := range []struct {
		name string
		dest *[]string
	}{
		{"languages", &c.Languages},
		{"rpc", &c.RPCLibraries},
		{"tsImportBoundaries", &c.TsImportBoundaries},
	} {
		list, err := configStrings(value, field.name)
		if err != nil {
			return err
		}
		*field.dest = list
	}

	// Presence enables the Rust graph even when the object contains only defaults.
	rust := value.Get("rust")
	if rust == nil || rust.Type() == fastjson.TypeNull {
		return nil
	}
	c.Rust = &RustConfig{}
	return c.Rust.FromJSON(rust)
}

// FromJSON reads the options and relative output paths for whole-graph Rust generation.
func (c *RustConfig) FromJSON(value *fastjson.Value) error {
	// Rust configuration is an object; a scalar cannot enable generation.
	if _, err := value.Object(); err != nil {
		return errors.Wrap(err, "aptre.rust")
	}

	// Each output path is optional; its absence keeps the documented default.
	for _, field := range []struct {
		name string
		dest *string
	}{
		{"descriptorSet", &c.DescriptorSet},
		{"moduleFile", &c.ModuleFile},
		{"inventory", &c.Inventory},
	} {
		text, err := configString(value, field.name)
		if err != nil {
			return err
		}
		*field.dest = text
	}

	// Plugin options and schema exclusions retain their declared order.
	for _, field := range []struct {
		name string
		dest *[]string
	}{
		{"prostOptions", &c.ProstOptions},
		{"exclude", &c.Exclude},
	} {
		list, err := configStrings(value, field.name)
		if err != nil {
			return err
		}
		*field.dest = list
	}
	return nil
}

// configString reads an optional string, treating null as absent.
func configString(value *fastjson.Value, key string) (string, error) {
	// Missing values preserve the configuration's zero-value defaults.
	field := value.Get(key)
	if field == nil || field.Type() == fastjson.TypeNull {
		return "", nil
	}

	// Copy the parsed text so configuration lifetime does not retain parser storage.
	text, err := field.StringBytes()
	return string(text), errors.Wrap(err, key)
}

// configStrings reads an optional string array, rejecting incorrectly typed entries.
func configStrings(value *fastjson.Value, key string) ([]string, error) {
	// Missing lists keep the default language and generator selection.
	field := value.Get(key)
	if field == nil || field.Type() == fastjson.TypeNull {
		return nil, nil
	}
	array, err := field.Array()
	if err != nil {
		return nil, errors.Wrap(err, key)
	}

	// Decode every entry before exposing the configured list.
	list := make([]string, 0, len(array))
	for _, item := range array {
		if item.Type() == fastjson.TypeNull {
			list = append(list, "")
			continue
		}
		text, err := item.StringBytes()
		if err != nil {
			return nil, errors.Wrap(err, key)
		}
		list = append(list, string(text))
	}
	return list, nil
}
