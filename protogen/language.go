package protogen

import "github.com/pkg/errors"

// Language identifies a protobuf output language.
type Language string

const (
	// LanguageGo enables Go protobuf outputs.
	LanguageGo Language = "go"
	// LanguageTypeScript enables TypeScript protobuf outputs.
	LanguageTypeScript Language = "ts"
	// LanguageCpp enables C++ protobuf outputs.
	LanguageCpp Language = "cpp"
	// LanguageRust enables Rust protobuf outputs.
	LanguageRust Language = "rust"
)

// Languages contains the enabled protobuf output languages.
type Languages map[Language]struct{}

// NewLanguages validates and normalizes output language names.
// Empty input preserves the historical default of all languages.
func NewLanguages(names []string) (Languages, error) {
	if len(names) == 0 {
		return Languages{
			LanguageGo:         {},
			LanguageTypeScript: {},
			LanguageCpp:        {},
			LanguageRust:       {},
		}, nil
	}

	langs := make(Languages, len(names))
	for _, name := range names {
		lang := Language(name)
		switch lang {
		case LanguageGo, LanguageTypeScript, LanguageCpp, LanguageRust:
			langs[lang] = struct{}{}
		default:
			return nil, errors.Errorf("unknown output language %q", name)
		}
	}
	return langs, nil
}

// Has returns true when the language is enabled.
func (l Languages) Has(lang Language) bool {
	if l == nil {
		return true
	}
	_, ok := l[lang]
	return ok
}
