package protogen

import (
	"strconv"
	"strings"

	liteprotogen "github.com/aperturerobotics/protobuf-go-lite/compiler/protogen"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

// TypeIDPluginName is the protoc plugin executable name served in process.
const TypeIDPluginName = "protoc-gen-go-typeid"

// typeIDOptionField is the field number of the (typeid.id) message option.
const typeIDOptionField protowire.Number = 51700

// typeIDPackage is the Go package declaring typeid.Type.
const typeIDPackage = liteprotogen.GoImportPath("github.com/aperturerobotics/common/typeid")

// typedMessage is a message that declares a (typeid.id) option.
type typedMessage struct {
	// msg is the generated Go message.
	msg *liteprotogen.Message
	// id is the declared type identifier.
	id string
}

// GenerateTypeIDs answers one encoded CodeGeneratorRequest for the typeid
// generator. For each proto file with typed messages it emits
// <name>_typeid.pb.go holding a TypeID constant and method per message and
// the file's []typeid.Type list.
func GenerateTypeIDs(input []byte) ([]byte, error) {
	// Decode the request into the Go generator's file model.
	req := &pluginpb.CodeGeneratorRequest{}
	if err := proto.Unmarshal(input, req); err != nil {
		return nil, err
	}
	gen, err := liteprotogen.Options{}.New(req)
	if err != nil {
		return nil, err
	}
	gen.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL |
		pluginpb.CodeGeneratorResponse_FEATURE_SUPPORTS_EDITIONS)
	gen.SupportedEditionsMinimum = descriptorpb.Edition_EDITION_PROTO2
	gen.SupportedEditionsMaximum = descriptorpb.Edition_EDITION_2024

	// Emit one file per requested proto that declares a type identifier.
	for _, file := range gen.Files {
		if !file.Generate {
			continue
		}
		types, err := fileTypedMessages(file)
		if err != nil {
			gen.Error(err)
			break
		}
		if len(types) != 0 {
			generateTypeIDFile(gen, file, types)
		}
	}

	return proto.Marshal(gen.Response())
}

// fileTypedMessages lists the typed messages of a file in declaration order and
// rejects an identifier declared twice.
func fileTypedMessages(file *liteprotogen.File) ([]typedMessage, error) {
	// Collect the typed messages of the whole nested message tree.
	types, err := appendTypedMessages(nil, file.Messages)
	if err != nil {
		return nil, err
	}

	// A type identifier names exactly one message.
	seen := make(map[string]string, len(types))
	for _, t := range types {
		name := string(t.msg.Desc.FullName())
		if prev, ok := seen[t.id]; ok {
			return nil, errors.Errorf("%s: type id %q already declared by %s", name, t.id, prev)
		}
		seen[t.id] = name
	}
	return types, nil
}

// appendTypedMessages appends each typed message of msgs and their nested
// messages, depth first in declaration order.
func appendTypedMessages(types []typedMessage, msgs []*liteprotogen.Message) ([]typedMessage, error) {
	for _, msg := range msgs {
		// Read the message's own option before its nested messages.
		id, ok, err := messageTypeID(msg)
		if err != nil {
			return nil, errors.Wrap(err, string(msg.Desc.FullName()))
		}
		if ok {
			types = append(types, typedMessage{msg: msg, id: id})
		}

		// Nested messages follow their parent.
		types, err = appendTypedMessages(types, msg.Messages)
		if err != nil {
			return nil, err
		}
	}
	return types, nil
}

// messageTypeID reads the (typeid.id) option of a message.
//
// The option is read from the encoded MessageOptions, so the generator needs
// no registered extension and reads it whether protoc's descriptor resolved
// the extension or left it as an unknown field.
func messageTypeID(msg *liteprotogen.Message) (string, bool, error) {
	// Encode the options as they appear on the wire.
	b, err := proto.Marshal(msg.Desc.Options())
	if err != nil {
		return "", false, err
	}

	// Scan for the option field; the last occurrence wins, as in protobuf.
	var id string
	var found bool
	for len(b) != 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return "", false, protowire.ParseError(n)
		}
		b = b[n:]
		if num == typeIDOptionField && typ == protowire.BytesType {
			v, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return "", false, protowire.ParseError(n)
			}
			id, found = string(v), true
			b = b[n:]
			continue
		}
		n = protowire.ConsumeFieldValue(num, typ, b)
		if n < 0 {
			return "", false, protowire.ParseError(n)
		}
		b = b[n:]
	}

	// A declared identifier must be usable as a key.
	if found && id == "" {
		return "", false, errors.New("type id is empty")
	}
	return id, found, nil
}

// generateTypeIDFile writes the TypeID declarations and list of one file.
func generateTypeIDFile(gen *liteprotogen.Plugin, file *liteprotogen.File, types []typedMessage) {
	// Open the output beside the file's message declarations.
	g := gen.NewGeneratedFile(file.GeneratedFilenamePrefix+"_typeid.pb.go", file.GoImportPath)
	g.P("// Code generated by ", TypeIDPluginName, ". DO NOT EDIT.")
	g.P("// source: ", file.Desc.Path())
	g.P()
	g.P("package ", file.GoPackageName)
	g.P()

	// Declare each message's identifier and its TypeID method.
	for _, t := range types {
		name := t.msg.GoIdent.GoName
		g.P("// ", name, "TypeID is the type ID of ", name, ".")
		g.P("const ", name, "TypeID = ", strconv.Quote(t.id))
		g.P()
		g.P("// TypeID returns the type ID of ", name, ".")
		g.P("func (*", name, ") TypeID() string {")
		g.P("return ", name, "TypeID")
		g.P("}")
		g.P()
	}

	// List the file's typed messages with their constructors.
	listName := typeIDListName(file.Desc.Path())
	g.P("// ", listName, " lists the typed messages of ", baseName(file.Desc.Path()), ".")
	g.P("var ", listName, " = []", g.QualifiedGoIdent(typeIDPackage.Ident("Type")), "{")
	for _, t := range types {
		name := t.msg.GoIdent.GoName
		g.P("{ID: ", name, "TypeID, New: func() any { return new(", name, ") }},")
	}
	g.P("}")
}

// typeIDListName names a file's type list: example_ops.proto lists ExampleOpsTypes.
func typeIDListName(path string) string {
	// Capitalize each alphanumeric run of the file's base name.
	var name strings.Builder
	upper := true
	for _, c := range strings.TrimSuffix(baseName(path), ".proto") {
		isLower := c >= 'a' && c <= 'z'
		isAlnum := isLower || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
		switch {
		case !isAlnum:
			upper = true
		case upper && isLower:
			name.WriteRune(c - 'a' + 'A')
			upper = false
		default:
			name.WriteRune(c)
			upper = false
		}
	}
	return name.String() + "Types"
}

// baseName returns the last element of a slash-separated proto path.
func baseName(path string) string {
	return path[strings.LastIndexByte(path, '/')+1:]
}
