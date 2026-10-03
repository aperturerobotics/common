package protogen

import (
	"strings"
	"unicode"
)

// This file ports the module naming of prost-build 0.14 (ident.rs) and of the
// heck 0.5 snake case conversion it uses, so that the module declarations name
// each protobuf package exactly as the generated message files expect. The
// port is licensed as LICENSE.prost-build (Apache-2.0) and LICENSE.heck (MIT).

// wordMode is the case of the previous characters while splitting a word.
type wordMode int

const (
	// wordBoundary follows a word boundary, before any cased character.
	wordBoundary wordMode = iota
	// wordLowercase follows a lowercase character.
	wordLowercase
	// wordUppercase follows an uppercase character.
	wordUppercase
)

// splitWords splits s into words the way heck does: at every character that is
// not alphanumeric, where a lowercase character meets an uppercase one, and
// before the last uppercase character of a run that precedes a lowercase one.
func splitWords(s string) []string {
	var words []string
	segments := strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})

	for _, segment := range segments {
		runes := []rune(segment)
		start := 0
		mode := wordBoundary

		// Scan each character together with the one that follows it.
		for i, c := range runes {
			if i == len(runes)-1 {
				words = append(words, string(runes[start:]))
				break
			}

			next := runes[i+1]
			nextMode := mode
			switch {
			case unicode.IsLower(c):
				nextMode = wordLowercase
			case unicode.IsUpper(c):
				nextMode = wordUppercase
			}

			switch {
			case nextMode == wordLowercase && unicode.IsUpper(next):
				// A lowercase character is followed by an uppercase one.
				words = append(words, string(runes[start:i+1]))
				start = i + 1
				mode = wordBoundary
			case mode == wordUppercase && unicode.IsUpper(c) && unicode.IsLower(next):
				// The last uppercase character of a run starts the next word.
				words = append(words, string(runes[start:i]))
				start = i
				mode = wordBoundary
			default:
				mode = nextMode
			}
		}
	}
	return words
}

// rustModuleName returns the Rust module name prost gives to a protobuf package
// segment: the snake case of the segment, with a keyword made a raw identifier,
// a keyword that cannot be raw given a trailing underscore, and a leading digit
// given a leading underscore. Segments that differ only in case share a name.
func rustModuleName(segment string) string {
	// Prost first normalizes package segments to snake case.
	words := splitWords(segment)
	for i, word := range words {
		words[i] = strings.ToLower(word)
	}
	name := strings.Join(words, "_")

	// Rust keywords and leading digits need an identifier escape after normalization.
	switch name {
	case "as", "break", "const", "continue", "else", "enum", "false", "fn",
		"for", "if", "impl", "in", "let", "loop", "match", "mod", "move", "mut",
		"pub", "ref", "return", "static", "struct", "trait", "true", "type",
		"unsafe", "use", "where", "while", "dyn", "abstract", "become", "box",
		"do", "final", "macro", "override", "priv", "typeof", "unsized",
		"virtual", "yield", "async", "await", "try", "gen":
		return "r#" + name
	case "_", "super", "self", "Self", "extern", "crate":
		return name + "_"
	}
	if r := []rune(name); len(r) != 0 && unicode.IsNumber(r[0]) {
		return "_" + name
	}
	return name
}
