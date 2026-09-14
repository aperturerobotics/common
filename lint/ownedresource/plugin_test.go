package ownedresource

import (
	"testing"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis/analysistest"
)

// TestAnalyzer checks diagnostics and accepted ownership paths through real CFGs.
func TestAnalyzer(t *testing.T) {
	p := &Plugin{Resources: []Resource{
		{Type: "handles.Handle", ReleaseMethods: []string{"Release"}, Consumers: []string{"handles.Release:0", "handles.Consume:1", "handles.Sink.Take:0"}, Borrowed: []string{"handles.Borrow"}},
		{Type: "*handles.Pointer", ReleaseMethods: []string{"Close"}},
		{Type: "handles.Strict", ReleaseMethods: []string{"Release"}},
		{Type: "handles.Success", ReleaseMethods: []string{"Release"}, NilOnError: true},
	}}
	analyzers, err := p.BuildAnalyzers()
	if err != nil {
		t.Fatal(err)
	}
	analysistest.Run(t, analysistest.TestData(), analyzers[0], "cases")
}

// TestPlugin exercises the loader's settings decoder and contract validation.
func TestPlugin(t *testing.T) {
	constructor, err := register.GetPlugin("ownedresource")
	if err != nil {
		t.Fatal(err)
	}
	plugin, err := constructor(map[string]any{
		"check-paths": false,
		"resources":   []any{map[string]any{"type": "handles.Handle", "release-methods": []string{"Release"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plugin.GetLoadMode() != register.LoadModeTypesInfo {
		t.Fatal("plugin must request type information")
	}
	analyzers, err := plugin.BuildAnalyzers()
	if err != nil {
		t.Fatal(err)
	}
	analysistest.Run(t, analysistest.TestData(), analyzers[0], "discard")

	// A misspelled setting must fail instead of silently disabling a contract.
	if _, err := constructor(map[string]any{"resource": "handles.Handle"}); err == nil {
		t.Fatal("accepted an unknown setting")
	}
	for _, resources := range [][]Resource{
		nil,
		{{Type: "Handle", ReleaseMethods: []string{"Release"}}},
		{{Type: "handles.Handle"}},
		{{Type: "handles.Handle", Consumers: []string{"handles.Release"}}},
		{{Type: "handles.Handle", Consumers: []string{"handles.Release:-1"}}},
		{{Type: "handles.Handle", ReleaseMethods: []string{"Release"}}, {Type: "handles.Handle", ReleaseMethods: []string{"Release"}}},
	} {
		if _, err := (&Plugin{Resources: resources}).BuildAnalyzers(); err == nil {
			t.Fatalf("accepted invalid contracts: %+v", resources)
		}
	}
}
