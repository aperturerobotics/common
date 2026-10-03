package protogen

import (
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// RustModules declares generated Rust files under the module of their protobuf
// package. A package whose files sit in several directories gets one module, so
// each message is defined once and references between its files resolve.
// Modules are named as prost names them, so packages that prost merges into one
// module, such as Upper.case and upper.case, share one here.
type RustModules struct {
	// files are the project-relative paths included directly in this module.
	files []string
	// children are the modules of the next package segment, by Rust name.
	children map[string]*RustModules
}

// NewRustModules constructs an empty module tree.
func NewRustModules() *RustModules {
	return &RustModules{children: make(map[string]*RustModules)}
}

// Add includes file in the module of the dotted protobuf package.
// An empty package selects the root module.
func (m *RustModules) Add(protoPackage, file string) {
	node := m
	if protoPackage != "" {
		for segment := range strings.SplitSeq(protoPackage, ".") {
			name := rustModuleName(segment)
			child, ok := node.children[name]
			if !ok {
				child = NewRustModules()
				node.children[name] = child
			}
			node = child
		}
	}
	node.files = append(node.files, file)
}

// Render returns the module declarations. Includes are relative to the crate
// root through CARGO_MANIFEST_DIR, so the output does not depend on the
// checkout path. Output is sorted and so independent of the order of Add.
func (m *RustModules) Render() string {
	var out strings.Builder
	m.render(&out, 0)
	return out.String()
}

// render writes this module's includes, then its child modules, at depth.
func (m *RustModules) render(out *strings.Builder, depth int) {
	indent := strings.Repeat("    ", depth)

	// Declare this package's files before opening any child package.
	for _, file := range slices.Sorted(slices.Values(m.files)) {
		out.WriteString(indent + "include!(concat!(env!(\"CARGO_MANIFEST_DIR\"), " + rustStringLiteral("/"+file) + "));\n")
	}

	// Nest the child packages in name order.
	for _, name := range slices.Sorted(maps.Keys(m.children)) {
		out.WriteString(indent + "pub mod " + name + " {\n")
		m.children[name].render(out, depth+1)
		out.WriteString(indent + "}\n")
	}
}

// rustStringLiteral quotes s as a Rust string literal. Quotes, backslashes and
// control characters are escaped; other text is kept as UTF-8.
func rustStringLiteral(s string) string {
	// Rust uses brace-delimited Unicode escapes for control characters.
	var out strings.Builder
	out.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			out.WriteByte('\\')
			out.WriteRune(r)
		case r == '\n':
			out.WriteString(`\n`)
		case r == '\r':
			out.WriteString(`\r`)
		case r == '\t':
			out.WriteString(`\t`)
		case unicode.IsControl(r):
			out.WriteString(`\u{` + strconv.FormatInt(int64(r), 16) + `}`)
		default:
			out.WriteRune(r)
		}
	}
	out.WriteByte('"')
	return out.String()
}
