package cache

import "strings"

// RunKey identifies a language and source selection. languages must be sorted.
// Protoc options, including RPC plugins, invalidate this selection in place so
// generation can remove obsolete outputs.
func RunKey(modulePath, projectDir, moduleDir string, languages, targets, exclude []string) string {
	return HashProtocFlags([]string{
		modulePath,
		relativizePath(projectDir, moduleDir),
		strings.Join(languages, ","),
		HashProtocFlags(targets, ""),
		HashProtocFlags(exclude, ""),
	}, "")
}
