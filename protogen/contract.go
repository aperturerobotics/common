package protogen

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	contractDeclarationPattern = regexp.MustCompile(`^\s*(enum|message|service)\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
	enumValuePattern           = regexp.MustCompile(`^\s*([A-Z][A-Z0-9_]*)\s*=\s*([0-9]+)\b`)
	fieldPattern               = regexp.MustCompile(`^\s*(?:optional\s+|repeated\s+)?(?:map\s*<[^>]+>|[.A-Za-z_][.A-Za-z0-9_]*)\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*[0-9]+\b`)
	rpcPattern                 = regexp.MustCompile(`^\s*rpc\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
)

// CheckProtoContracts validates the mechanical house contract for first-party
// protobuf sources selected by an explicit strict generation request.
func CheckProtoContracts(projectDir string, protoFiles []string) error {
	var diagnostics []string
	for _, protoFile := range protoFiles {
		if !isFirstPartyProto(protoFile) {
			continue
		}
		path := protoFile
		if !filepath.IsAbs(path) {
			path = filepath.Join(projectDir, path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read proto contract source %s: %w", protoFile, err)
		}
		diagnostics = append(diagnostics, checkProtoContract(protoFile, string(data))...)
	}
	if len(diagnostics) != 0 {
		return fmt.Errorf("proto contract check failed:\n%s", strings.Join(diagnostics, "\n"))
	}
	return nil
}

func isFirstPartyProto(protoFile string) bool {
	clean := filepath.Clean(protoFile)
	for _, part := range strings.FieldsFunc(filepath.ToSlash(clean), func(r rune) bool { return r == '/' }) {
		if part == "vendor" {
			return false
		}
	}
	base := filepath.Base(clean)
	return !strings.HasSuffix(base, ".pb.proto") && !strings.HasSuffix(base, "_pb.proto")
}

type protoContractBlock struct {
	kind        string
	name        string
	parentDepth int
	zeroSeen    bool
}

func checkProtoContract(path, source string) []string {
	lines := strings.Split(source, "\n")
	proto3 := false
	for _, line := range lines {
		if strings.TrimSpace(strings.SplitN(line, "//", 2)[0]) == `syntax = "proto3";` {
			proto3 = true
			break
		}
	}

	var diagnostics []string
	var blocks []protoContractBlock
	var pending *protoContractBlock
	lastDeclarationEnd := make(map[int]int)
	inBlockComment := false
	for index, line := range lines {
		lineNumber := index + 1
		code := structuralCode(line, &inBlockComment)
		openedPending := pending != nil && strings.Contains(code, "{")
		if openedPending {
			blocks = append(blocks, *pending)
			pending = nil
		}
		declaration := contractDeclarationPattern.FindStringSubmatch(code)
		if declaration != nil {
			kind, name := declaration[1], declaration[2]
			parentDepth := len(blocks)
			if previousEnd, ok := lastDeclarationEnd[parentDepth]; ok && !hasBlankLine(lines, previousEnd, index) {
				diagnostics = append(diagnostics, contractDiagnostic(path, lineNumber, "%s %s requires a blank line after the preceding declaration", kind, name))
			}
			if !hasDocComment(lines, index) {
				diagnostics = append(diagnostics, contractDiagnostic(path, lineNumber, "%s %s requires a semantic comment immediately above it", kind, name))
			}
			if (kind == "message" || kind == "enum") && strings.Contains(code, "{") && strings.Contains(code, "}") {
				diagnostics = append(diagnostics, contractDiagnostic(path, lineNumber, "%s %s must use an expanded block", kind, name))
			}
		}

		nearest := nearestContractBlock(blocks)
		if nearest != nil && nearest.kind == "enum" {
			if value := enumValuePattern.FindStringSubmatch(code); value != nil {
				if !hasDocComment(lines, index) {
					diagnostics = append(diagnostics, contractDiagnostic(path, lineNumber, "enum value %s requires a semantic comment immediately above it", value[1]))
				}
				if value[2] == "0" {
					nearest.zeroSeen = true
					if proto3 && !strings.HasSuffix(value[1], "_UNKNOWN") {
						diagnostics = append(diagnostics, contractDiagnostic(path, lineNumber, "proto3 enum %s must declare a value ending in _UNKNOWN = 0", nearest.name))
					}
				}
			}
		}
		if nearest != nil && nearest.kind == "message" {
			if field := fieldPattern.FindStringSubmatch(code); field != nil {
				if !hasDocComment(lines, index) {
					diagnostics = append(diagnostics, contractDiagnostic(path, lineNumber, "field %s requires a semantic comment immediately above it", field[1]))
				}
				if strings.Count(code, ";") > 1 {
					diagnostics = append(diagnostics, contractDiagnostic(path, lineNumber, "field %s must be on its own line", field[1]))
				}
			}
		}
		if nearest != nil && nearest.kind == "service" {
			if rpc := rpcPattern.FindStringSubmatch(code); rpc != nil && !hasDocComment(lines, index) {
				diagnostics = append(diagnostics, contractDiagnostic(path, lineNumber, "RPC %s requires a semantic comment immediately above it", rpc[1]))
			}
		}

		openCount := strings.Count(code, "{")
		closeCount := strings.Count(code, "}")
		if openedPending {
			openCount--
		}
		if declaration != nil && openCount > 0 {
			blocks = append(blocks, protoContractBlock{kind: declaration[1], name: declaration[2], parentDepth: len(blocks)})
			openCount--
		} else if declaration != nil {
			pending = &protoContractBlock{kind: declaration[1], name: declaration[2], parentDepth: len(blocks)}
		}
		for range openCount {
			blocks = append(blocks, protoContractBlock{kind: "other"})
		}
		for range closeCount {
			if len(blocks) == 0 {
				continue
			}
			block := blocks[len(blocks)-1]
			blocks = blocks[:len(blocks)-1]
			if block.kind == "enum" && proto3 && !block.zeroSeen {
				diagnostics = append(diagnostics, contractDiagnostic(path, lineNumber, "proto3 enum %s must declare a value ending in _UNKNOWN = 0", block.name))
			}
			if block.kind == "enum" || block.kind == "message" || block.kind == "service" {
				lastDeclarationEnd[block.parentDepth] = index
			}
		}
	}
	return diagnostics
}

// structuralCode removes comments and quoted strings before brace accounting.
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

func nearestContractBlock(blocks []protoContractBlock) *protoContractBlock {
	for index := len(blocks) - 1; index >= 0; index-- {
		if blocks[index].kind != "other" {
			return &blocks[index]
		}
	}
	return nil
}

func hasDocComment(lines []string, index int) bool {
	if index == 0 {
		return false
	}
	comment := strings.TrimSpace(lines[index-1])
	return strings.HasPrefix(comment, "//") && strings.TrimSpace(strings.TrimPrefix(comment, "//")) != ""
}

func hasBlankLine(lines []string, start, end int) bool {
	for _, line := range lines[start+1 : end] {
		if strings.TrimSpace(line) == "" {
			return true
		}
	}
	return false
}

func contractDiagnostic(path string, line int, format string, args ...any) string {
	return fmt.Sprintf("%s:%d: %s", path, line, fmt.Sprintf(format, args...))
}
