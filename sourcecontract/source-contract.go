// Package sourcecontract checks mechanically decidable source contracts.
package sourcecontract

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/pkg/errors"
)

const generatedGoPattern = `^// Code generated .* DO NOT EDIT\.$`

var (
	generatedGoRegexp       = regexp.MustCompile(generatedGoPattern)
	protoDeclarationPattern = regexp.MustCompile(`^\s*(enum|message|service)\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
	protoExtendPattern      = regexp.MustCompile(`^\s*extend\s+[^\s{]+`)
	protoEnumValuePattern   = regexp.MustCompile(`\b([A-Z][A-Z0-9_]*)\s*=\s*([0-9]+)\b`)
	protoFieldPattern       = regexp.MustCompile(`(?:^|[;{]\s*)\s*(?:optional\s+|repeated\s+)?(?:map\s*<[^>]+>|[.A-Za-z_][.A-Za-z0-9_]*)\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*[0-9]+\b`)
	protoRPCPattern         = regexp.MustCompile(`\brpc\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
)

// Check checks the selected project source and returns deterministic diagnostics.
func Check(projectDir string) error {
	paths, err := projectSources(projectDir)
	if err != nil {
		return err
	}
	var diagnostics []diagnostic
	for _, path := range paths {
		if excludedPath(path) || (!strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, ".proto")) {
			continue
		}
		fullPath := filepath.Join(projectDir, filepath.FromSlash(path))
		info, err := os.Lstat(fullPath)
		if err != nil {
			return errors.Wrapf(err, "inspect source %s", path)
		}
		if !info.Mode().IsRegular() {
			diagnostics = append(diagnostics, newDiagnostic(path, 1, 1, "source-regular", "source candidate is nonregular"))
			continue
		}
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return errors.Wrapf(err, "read source %s", path)
		}
		if strings.HasSuffix(path, ".go") {
			if isGeneratedGo(data) {
				continue
			}
			diagnostics = append(diagnostics, checkGo(path, data)...)
			continue
		}
		base := filepath.Base(path)
		if strings.HasSuffix(base, ".pb.proto") || strings.HasSuffix(base, "_pb.proto") {
			continue
		}
		diagnostics = append(diagnostics, checkProto(path, string(data))...)
	}
	sort.Slice(diagnostics, func(i, j int) bool { return diagnostics[i].less(diagnostics[j]) })
	if len(diagnostics) == 0 {
		return nil
	}
	lines := make([]string, len(diagnostics))
	for index := range diagnostics {
		lines[index] = diagnostics[index].String()
	}
	return errors.New(strings.Join(lines, "\n"))
}

type diagnostic struct {
	path, rule, message string
	line, column        int
}

func newDiagnostic(path string, line, column int, rule, message string) diagnostic {
	return diagnostic{path: path, line: line, column: column, rule: rule, message: message}
}

func (d diagnostic) String() string {
	return d.path + ":" + strconv.Itoa(d.line) + ":" + strconv.Itoa(d.column) + ": " + d.rule + ": " + d.message
}

func (d diagnostic) less(other diagnostic) bool {
	if order := bytes.Compare([]byte(d.path), []byte(other.path)); order != 0 {
		return order < 0
	}
	if d.line != other.line {
		return d.line < other.line
	}
	if d.column != other.column {
		return d.column < other.column
	}
	return d.rule < other.rule
}

func projectSources(projectDir string) ([]string, error) {
	cmd := exec.Command("git", "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	cmd.Dir = projectDir
	output, err := cmd.Output()
	if err != nil {
		return nil, errors.Wrap(err, "list project source")
	}
	records := bytes.Split(output, []byte{0})
	paths := make([]string, 0, len(records))
	for _, record := range records {
		if len(record) != 0 {
			paths = append(paths, string(record))
		}
	}
	sort.Slice(paths, func(i, j int) bool { return bytes.Compare([]byte(paths[i]), []byte(paths[j])) < 0 })
	return paths, nil
}

func excludedPath(path string) bool {
	for _, component := range strings.Split(filepath.ToSlash(path), "/") {
		if component == "vendor" || component == "node_modules" || component == ".tools" {
			return true
		}
	}
	return false
}

func isGeneratedGo(data []byte) bool {
	lines := bytes.Split(data, []byte("\n"))
	if len(lines) > 20 {
		lines = lines[:20]
	}
	for _, line := range lines {
		if generatedGoRegexp.Match(line) {
			return true
		}
	}
	return false
}

func checkGo(path string, data []byte) []diagnostic {
	files := token.NewFileSet()
	file, err := parser.ParseFile(files, path, data, parser.ParseComments)
	if err != nil {
		return []diagnostic{newDiagnostic(path, 1, 1, "go-parse", err.Error())}
	}
	var diagnostics []diagnostic
	for _, declaration := range file.Decls {
		switch typed := declaration.(type) {
		case *ast.FuncDecl:
			diagnostics = append(diagnostics, goCommentDiagnostic(files, path, typed.Name, typed.Doc, "go-declaration")...)
		case *ast.GenDecl:
			for _, specification := range typed.Specs {
				switch spec := specification.(type) {
				case *ast.TypeSpec:
					comment := spec.Doc
					if comment == nil && len(typed.Specs) == 1 {
						comment = typed.Doc
					}
					diagnostics = append(diagnostics, goCommentDiagnostic(files, path, spec.Name, comment, "go-declaration")...)
					ast.Inspect(spec.Type, func(node ast.Node) bool {
						structure, ok := node.(*ast.StructType)
						if !ok {
							return true
						}
						for _, field := range structure.Fields.List {
							for _, name := range field.Names {
								diagnostics = append(diagnostics, goCommentDiagnostic(files, path, name, field.Doc, "go-field")...)
							}
						}
						return true
					})
				case *ast.ValueSpec:
					comment := spec.Doc
					if comment == nil && len(typed.Specs) == 1 {
						comment = typed.Doc
					}
					for _, name := range spec.Names {
						diagnostics = append(diagnostics, goCommentDiagnostic(files, path, name, comment, "go-declaration")...)
					}
				}
			}
		}
	}
	return diagnostics
}

func goCommentDiagnostic(files *token.FileSet, path string, name *ast.Ident, comment *ast.CommentGroup, rule string) []diagnostic {
	position := files.Position(name.Pos())
	if validGoComment(files, name.Name, position.Line, comment) {
		return nil
	}
	return []diagnostic{newDiagnostic(path, position.Line, position.Column, rule, name.Name+" requires an adjacent identifier-first // comment ending in a period")}
}

func validGoComment(files *token.FileSet, name string, line int, comment *ast.CommentGroup) bool {
	if comment == nil || files.Position(comment.End()).Line != line-1 {
		return false
	}
	for _, item := range comment.List {
		if !strings.HasPrefix(item.Text, "//") {
			return false
		}
	}
	text := strings.TrimSpace(comment.Text())
	return strings.HasPrefix(text, name+" ") && strings.HasSuffix(text, ".")
}

type protoBlock struct {
	kind, name  string
	parentDepth int
	firstValue  bool
}

func checkProto(path, source string) []diagnostic {
	lines := strings.Split(source, "\n")
	proto3 := false
	for _, line := range lines {
		if strings.TrimSpace(strings.SplitN(line, "//", 2)[0]) == `syntax = "proto3";` {
			proto3 = true
		}
	}
	var diagnostics []diagnostic
	var blocks []protoBlock
	var pending *protoBlock
	lastTopEnd := -1
	inBlockComment := false
	for index, line := range lines {
		lineNumber := index + 1
		code := structuralCode(line, &inBlockComment)
		openedPending := pending != nil && strings.Contains(code, "{")
		if openedPending {
			blocks = append(blocks, *pending)
			pending = nil
		}
		declaration := protoDeclarationPattern.FindStringSubmatchIndex(code)
		if declaration != nil {
			kind := code[declaration[2]:declaration[3]]
			name := code[declaration[4]:declaration[5]]
			column := declaration[4] + 1
			if len(blocks) == 0 && lastTopEnd >= 0 && blankLineCount(lines, lastTopEnd, index) != 1 {
				diagnostics = append(diagnostics, newDiagnostic(path, lineNumber, column, "proto-separation", name+" requires exactly one blank line after the preceding declaration"))
			}
			if !validProtoComment(lines, index, name) {
				diagnostics = append(diagnostics, newDiagnostic(path, lineNumber, column, "proto-comment", name+" requires an adjacent identifier-first // comment ending in a period"))
			}
			if (kind == "message" || kind == "enum") && strings.Contains(code, "{") && strings.Contains(code, "}") {
				diagnostics = append(diagnostics, newDiagnostic(path, lineNumber, column, "proto-expanded", name+" must use an expanded block"))
			}
			if kind == "service" && protoRPCPattern.MatchString(code) {
				diagnostics = append(diagnostics, newDiagnostic(path, lineNumber, column, "proto-rpc-line", "RPC must follow its comment on its own line"))
			}
		}

		nearest := nearestProtoBlock(blocks)
		var inline protoBlock
		if declaration != nil && strings.Contains(code, "{") {
			inline = protoBlock{kind: code[declaration[2]:declaration[3]], name: code[declaration[4]:declaration[5]], firstValue: true}
			nearest = &inline
		} else if protoExtendPattern.MatchString(code) && strings.Contains(code, "{") {
			inline = protoBlock{kind: "extend"}
			nearest = &inline
		}
		for _, match := range protoEnumValuePattern.FindAllStringSubmatchIndex(code, -1) {
			if nearest == nil || nearest.kind != "enum" {
				continue
			}
			name := code[match[2]:match[3]]
			value := code[match[4]:match[5]]
			column := match[2] + 1
			if !validProtoComment(lines, index, name) {
				diagnostics = append(diagnostics, newDiagnostic(path, lineNumber, column, "proto-comment", name+" requires an adjacent identifier-first // comment ending in a period"))
			}
			if nearest.firstValue {
				nearest.firstValue = false
				if proto3 && (name != screamingSnake(nearest.name)+"_UNKNOWN" || value != "0") {
					diagnostics = append(diagnostics, newDiagnostic(path, lineNumber, column, "proto-enum-zero", nearest.name+" must start with its enum-prefixed UNKNOWN = 0 value"))
				}
			}
		}
		for _, match := range protoFieldPattern.FindAllStringSubmatchIndex(code, -1) {
			if nearest == nil || (nearest.kind != "message" && nearest.kind != "extend") {
				continue
			}
			name := code[match[2]:match[3]]
			column := match[2] + 1
			publicName := protoPublicName(name)
			if !validProtoComment(lines, index, publicName) {
				diagnostics = append(diagnostics, newDiagnostic(path, lineNumber, column, "proto-comment", publicName+" requires an adjacent identifier-first // comment ending in a period"))
			}
			if strings.Count(code, ";") > 1 {
				diagnostics = append(diagnostics, newDiagnostic(path, lineNumber, column, "proto-field-line", publicName+" must follow its comment on its own line"))
			}
		}
		for _, match := range protoRPCPattern.FindAllStringSubmatchIndex(code, -1) {
			if nearest == nil || nearest.kind != "service" {
				continue
			}
			name := code[match[2]:match[3]]
			column := match[2] + 1
			if !validProtoComment(lines, index, name) {
				diagnostics = append(diagnostics, newDiagnostic(path, lineNumber, column, "proto-comment", name+" requires an adjacent identifier-first // comment ending in a period"))
			}
			if strings.Count(code, ";") > 1 {
				diagnostics = append(diagnostics, newDiagnostic(path, lineNumber, column, "proto-rpc-line", name+" must follow its comment on its own line"))
			}
		}

		openCount := strings.Count(code, "{")
		closeCount := strings.Count(code, "}")
		if openedPending {
			openCount--
		}
		if declaration != nil && openCount > 0 {
			blocks = append(blocks, protoBlock{kind: code[declaration[2]:declaration[3]], name: code[declaration[4]:declaration[5]], parentDepth: len(blocks), firstValue: !protoEnumValuePattern.MatchString(code)})
			openCount--
		} else if declaration != nil {
			pending = &protoBlock{kind: code[declaration[2]:declaration[3]], name: code[declaration[4]:declaration[5]], parentDepth: len(blocks), firstValue: true}
		} else if protoExtendPattern.MatchString(code) && openCount > 0 {
			blocks = append(blocks, protoBlock{kind: "extend", parentDepth: len(blocks)})
			openCount--
		} else if protoExtendPattern.MatchString(code) {
			pending = &protoBlock{kind: "extend", parentDepth: len(blocks)}
		}
		for range openCount {
			blocks = append(blocks, protoBlock{kind: "other"})
		}
		for range closeCount {
			if len(blocks) == 0 {
				continue
			}
			block := blocks[len(blocks)-1]
			blocks = blocks[:len(blocks)-1]
			if block.kind == "enum" && proto3 && block.firstValue {
				diagnostics = append(diagnostics, newDiagnostic(path, lineNumber, 1, "proto-enum-zero", block.name+" must start with its enum-prefixed UNKNOWN = 0 value"))
			}
			if block.parentDepth == 0 && (block.kind == "enum" || block.kind == "message" || block.kind == "service") {
				lastTopEnd = index
			}
		}
	}
	return diagnostics
}

func structuralCode(line string, inBlockComment *bool) string {
	var code strings.Builder
	for index := 0; index < len(line); {
		if *inBlockComment {
			if index+1 < len(line) && line[index:index+2] == "*/" {
				*inBlockComment = false
				index += 2
			} else {
				index++
			}
			continue
		}
		if index+1 < len(line) && line[index:index+2] == "//" {
			break
		}
		if index+1 < len(line) && line[index:index+2] == "/*" {
			*inBlockComment = true
			index += 2
			continue
		}
		if line[index] == '"' || line[index] == '\'' {
			quote := line[index]
			index++
			for index < len(line) {
				if line[index] == '\\' {
					index += 2
					continue
				}
				if line[index] == quote {
					index++
					break
				}
				index++
			}
			continue
		}
		code.WriteByte(line[index])
		index++
	}
	return code.String()
}

func nearestProtoBlock(blocks []protoBlock) *protoBlock {
	for index := len(blocks) - 1; index >= 0; index-- {
		if blocks[index].kind != "other" {
			return &blocks[index]
		}
	}
	return nil
}

func validProtoComment(lines []string, index int, name string) bool {
	if index == 0 || !strings.HasPrefix(strings.TrimSpace(lines[index-1]), "//") {
		return false
	}
	start := index - 1
	for start > 0 && strings.HasPrefix(strings.TrimSpace(lines[start-1]), "//") {
		start--
	}
	first := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[start]), "//"))
	last := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[index-1]), "//"))
	return strings.HasPrefix(first, name+" ") && strings.HasSuffix(last, ".")
}

func blankLineCount(lines []string, start, end int) int {
	count := 0
	for _, line := range lines[start+1 : end] {
		if strings.TrimSpace(line) == "" {
			count++
		}
	}
	return count
}

func protoPublicName(name string) string {
	var result strings.Builder
	upper := true
	for _, r := range name {
		if r == '_' {
			upper = true
			continue
		}
		if upper {
			r = unicode.ToUpper(r)
			upper = false
		}
		result.WriteRune(r)
	}
	return result.String()
}

func screamingSnake(name string) string {
	runes := []rune(name)
	var result strings.Builder
	for index, current := range runes {
		if current == '_' {
			if result.Len() != 0 {
				result.WriteByte('_')
			}
			continue
		}
		if index > 0 && currentWordStarts(runes, index) {
			result.WriteByte('_')
		}
		result.WriteRune(unicode.ToUpper(current))
	}
	return result.String()
}

func currentWordStarts(runes []rune, index int) bool {
	current := runes[index]
	previous := runes[index-1]
	if unicode.IsUpper(current) {
		return unicode.IsLower(previous) || unicode.IsDigit(previous) ||
			(unicode.IsUpper(previous) && index+1 < len(runes) && unicode.IsLower(runes[index+1]))
	}
	return unicode.IsDigit(current) && !unicode.IsDigit(previous)
}
