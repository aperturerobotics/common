// protoc-gen-aptre-doc generates per-file .pb.json and .pb.md documentation
// from proto files using SourceCodeInfo comments.
package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aperturerobotics/protobuf-go-lite/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/pluginpb"
)

func main() {
	opts := protogen.Options{}
	opts.Run(func(plugin *protogen.Plugin) error {
		for _, f := range plugin.Files {
			if !f.Generate {
				continue
			}
			if err := generateJSON(plugin, f); err != nil {
				return err
			}
			generateMarkdown(plugin, f)
		}
		plugin.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)
		return nil
	})
}

// JSON schema types.

// FileDoc is the top-level JSON documentation for a proto file.
type FileDoc struct {
	Name        string       `json:"name"`
	Package     string       `json:"package"`
	Syntax      string       `json:"syntax"`
	Description string       `json:"description,omitempty"`
	Messages    []MessageDoc `json:"messages,omitempty"`
	Enums       []EnumDoc    `json:"enums,omitempty"`
	Services    []ServiceDoc `json:"services,omitempty"`
}

// MessageDoc documents a proto message.
type MessageDoc struct {
	Name        string       `json:"name"`
	FullName    string       `json:"fullName"`
	Description string       `json:"description,omitempty"`
	Fields      []FieldDoc   `json:"fields,omitempty"`
	Messages    []MessageDoc `json:"messages,omitempty"`
	Enums       []EnumDoc    `json:"enums,omitempty"`
}

// FieldDoc documents a proto field.
type FieldDoc struct {
	Name        string `json:"name"`
	JSONName    string `json:"jsonName"`
	Number      int    `json:"number"`
	Type        string `json:"type"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
	Default     string `json:"default,omitempty"`
}

// EnumDoc documents a proto enum.
type EnumDoc struct {
	Name        string         `json:"name"`
	FullName    string         `json:"fullName"`
	Description string         `json:"description,omitempty"`
	Values      []EnumValueDoc `json:"values,omitempty"`
}

// EnumValueDoc documents a proto enum value.
type EnumValueDoc struct {
	Name        string `json:"name"`
	Number      int    `json:"number"`
	Description string `json:"description,omitempty"`
}

// ServiceDoc documents a proto service.
type ServiceDoc struct {
	Name        string      `json:"name"`
	FullName    string      `json:"fullName"`
	Description string      `json:"description,omitempty"`
	Methods     []MethodDoc `json:"methods,omitempty"`
}

// MethodDoc documents a proto service method.
type MethodDoc struct {
	Name            string `json:"name"`
	Description     string `json:"description,omitempty"`
	InputType       string `json:"inputType"`
	OutputType      string `json:"outputType"`
	ClientStreaming  bool   `json:"clientStreaming,omitempty"`
	ServerStreaming  bool   `json:"serverStreaming,omitempty"`
}

// fileLeadingComment extracts the file-level leading comment from SourceCodeInfo.
// The file-level comment is at the syntax declaration (path [12]).
func fileLeadingComment(f *protogen.File) string {
	loc := f.Desc.SourceLocations().ByPath(protoreflect.SourcePath{12})
	return cleanCommentStr(loc.LeadingComments)
}

func generateJSON(plugin *protogen.Plugin, f *protogen.File) error {
	doc := FileDoc{
		Name:        f.Desc.Path(),
		Package:     string(f.Desc.Package()),
		Syntax:      f.Desc.Syntax().String(),
		Description: fileLeadingComment(f),
	}

	for _, msg := range f.Messages {
		doc.Messages = append(doc.Messages, buildMessageDoc(msg))
	}
	for _, enum := range f.Enums {
		doc.Enums = append(doc.Enums, buildEnumDoc(enum))
	}
	for _, svc := range f.Services {
		doc.Services = append(doc.Services, buildServiceDoc(svc))
	}

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}

	gf := plugin.NewGeneratedFile(f.GeneratedFilenamePrefix+".pb.json", "")
	gf.P(string(data))
	return nil
}

func generateMarkdown(plugin *protogen.Plugin, f *protogen.File) {
	gf := plugin.NewGeneratedFile(f.GeneratedFilenamePrefix+".pb.md", "")

	// Header
	gf.P("# ", f.Desc.Path())
	gf.P()
	if desc := fileLeadingComment(f); desc != "" {
		gf.P(desc)
		gf.P()
	}
	gf.P("**Package:** `", f.Desc.Package(), "`  ")
	gf.P("**Syntax:** `", f.Desc.Syntax().String(), "`")
	gf.P()

	// Messages
	if len(f.Messages) > 0 {
		gf.P("## Messages")
		gf.P()
		for _, msg := range f.Messages {
			writeMessageMarkdown(gf, msg, 3)
		}
	}

	// Enums
	if len(f.Enums) > 0 {
		gf.P("## Enums")
		gf.P()
		for _, enum := range f.Enums {
			writeEnumMarkdown(gf, enum, 3)
		}
	}

	// Services
	if len(f.Services) > 0 {
		gf.P("## Services")
		gf.P()
		for _, svc := range f.Services {
			writeServiceMarkdown(gf, svc)
		}
	}
}

func buildMessageDoc(msg *protogen.Message) MessageDoc {
	doc := MessageDoc{
		Name:        string(msg.Desc.Name()),
		FullName:    string(msg.Desc.FullName()),
		Description: cleanComment(msg.Comments.Leading),
	}
	for _, field := range msg.Fields {
		doc.Fields = append(doc.Fields, buildFieldDoc(field))
	}
	for _, nested := range msg.Messages {
		doc.Messages = append(doc.Messages, buildMessageDoc(nested))
	}
	for _, enum := range msg.Enums {
		doc.Enums = append(doc.Enums, buildEnumDoc(enum))
	}
	return doc
}

func buildFieldDoc(field *protogen.Field) FieldDoc {
	typeName := fieldTypeName(field)
	label := ""
	if field.Desc.IsList() {
		label = "repeated"
	} else if field.Desc.IsMap() {
		label = "map"
	} else if field.Desc.HasOptionalKeyword() {
		label = "optional"
	}

	doc := FieldDoc{
		Name:        string(field.Desc.Name()),
		JSONName:    field.Desc.JSONName(),
		Number:      int(field.Desc.Number()),
		Type:        typeName,
		Label:       label,
		Description: cleanComment(field.Comments.Leading),
	}

	if field.Desc.HasDefault() {
		doc.Default = field.Desc.Default().String()
	}
	return doc
}

func buildEnumDoc(enum *protogen.Enum) EnumDoc {
	doc := EnumDoc{
		Name:        string(enum.Desc.Name()),
		FullName:    string(enum.Desc.FullName()),
		Description: cleanComment(enum.Comments.Leading),
	}
	for _, val := range enum.Values {
		doc.Values = append(doc.Values, EnumValueDoc{
			Name:        string(val.Desc.Name()),
			Number:      int(val.Desc.Number()),
			Description: cleanComment(val.Comments.Leading),
		})
	}
	return doc
}

func buildServiceDoc(svc *protogen.Service) ServiceDoc {
	doc := ServiceDoc{
		Name:        string(svc.Desc.Name()),
		FullName:    string(svc.Desc.FullName()),
		Description: cleanComment(svc.Comments.Leading),
	}
	for _, method := range svc.Methods {
		doc.Methods = append(doc.Methods, MethodDoc{
			Name:            string(method.Desc.Name()),
			Description:     cleanComment(method.Comments.Leading),
			InputType:       string(method.Input.Desc.FullName()),
			OutputType:      string(method.Output.Desc.FullName()),
			ClientStreaming:  method.Desc.IsStreamingClient(),
			ServerStreaming:  method.Desc.IsStreamingServer(),
		})
	}
	return doc
}

func writeMessageMarkdown(gf *protogen.GeneratedFile, msg *protogen.Message, depth int) {
	heading := strings.Repeat("#", depth)
	gf.P(heading, " ", msg.Desc.Name())
	gf.P()
	if desc := cleanComment(msg.Comments.Leading); desc != "" {
		gf.P(desc)
		gf.P()
	}

	if len(msg.Fields) > 0 {
		gf.P("| Field | Type | Label | Description |")
		gf.P("|-------|------|-------|-------------|")
		for _, field := range msg.Fields {
			label := ""
			if field.Desc.IsList() {
				label = "repeated"
			} else if field.Desc.IsMap() {
				label = "map"
			} else if field.Desc.HasOptionalKeyword() {
				label = "optional"
			}
			desc := strings.ReplaceAll(cleanComment(field.Comments.Leading), "\n", " ")
			gf.P("| `", field.Desc.Name(), "` | `", fieldTypeName(field), "` | ", label, " | ", desc, " |")
		}
		gf.P()
	}

	for _, nested := range msg.Messages {
		writeMessageMarkdown(gf, nested, depth+1)
	}
	for _, enum := range msg.Enums {
		writeEnumMarkdown(gf, enum, depth+1)
	}
}

func writeEnumMarkdown(gf *protogen.GeneratedFile, enum *protogen.Enum, depth int) {
	heading := strings.Repeat("#", depth)
	gf.P(heading, " ", enum.Desc.Name())
	gf.P()
	if desc := cleanComment(enum.Comments.Leading); desc != "" {
		gf.P(desc)
		gf.P()
	}

	gf.P("| Name | Number | Description |")
	gf.P("|------|--------|-------------|")
	for _, val := range enum.Values {
		desc := strings.ReplaceAll(cleanComment(val.Comments.Leading), "\n", " ")
		gf.P("| `", val.Desc.Name(), "` | ", fmt.Sprintf("%d", val.Desc.Number()), " | ", desc, " |")
	}
	gf.P()
}

func writeServiceMarkdown(gf *protogen.GeneratedFile, svc *protogen.Service) {
	gf.P("### ", svc.Desc.Name())
	gf.P()
	if desc := cleanComment(svc.Comments.Leading); desc != "" {
		gf.P(desc)
		gf.P()
	}

	gf.P("| Method | Request | Response | Description |")
	gf.P("|--------|---------|----------|-------------|")
	for _, method := range svc.Methods {
		desc := strings.ReplaceAll(cleanComment(method.Comments.Leading), "\n", " ")
		inputName := string(method.Input.Desc.Name())
		outputName := string(method.Output.Desc.Name())
		if method.Desc.IsStreamingClient() {
			inputName = "stream " + inputName
		}
		if method.Desc.IsStreamingServer() {
			outputName = "stream " + outputName
		}
		gf.P("| `", method.Desc.Name(), "` | `", inputName, "` | `", outputName, "` | ", desc, " |")
	}
	gf.P()
}

// fieldTypeName returns a readable type name for a field.
func fieldTypeName(field *protogen.Field) string {
	if field.Desc.IsMap() {
		key := field.Desc.MapKey()
		val := field.Desc.MapValue()
		keyType := key.Kind().String()
		valType := val.Kind().String()
		if val.Kind() == protoreflect.MessageKind || val.Kind() == protoreflect.GroupKind {
			valType = string(val.Message().Name())
		}
		if val.Kind() == protoreflect.EnumKind {
			valType = string(val.Enum().Name())
		}
		return fmt.Sprintf("map<%s, %s>", keyType, valType)
	}

	kind := field.Desc.Kind()
	switch kind {
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return string(field.Desc.Message().Name())
	case protoreflect.EnumKind:
		return string(field.Desc.Enum().Name())
	default:
		return kind.String()
	}
}

// cleanComment strips comment prefixes and trims whitespace.
func cleanComment(c protogen.Comments) string {
	return cleanCommentStr(string(c))
}

// cleanCommentStr strips comment prefixes and trims whitespace from a string.
func cleanCommentStr(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Remove leading "// " from each line
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimPrefix(line, "// ")
		line = strings.TrimPrefix(line, "//")
		lines = append(lines, line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
