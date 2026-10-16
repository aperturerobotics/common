package protogen

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

// typedMessageProto builds a message descriptor with an optional type id.
func typedMessageProto(name, id string, nested ...*descriptorpb.DescriptorProto) *descriptorpb.DescriptorProto {
	// Messages without an id carry no options at all.
	msg := &descriptorpb.DescriptorProto{Name: proto.String(name), NestedType: nested}
	if id == "" {
		return msg
	}

	// Encode the option as protoc leaves an unregistered extension.
	opts := &descriptorpb.MessageOptions{}
	raw := protowire.AppendTag(nil, typeIDOptionField, protowire.BytesType)
	opts.ProtoReflect().SetUnknown(protowire.AppendString(raw, id))
	msg.Options = opts
	return msg
}

// runTypeIDGenerator runs GenerateTypeIDs over one file holding msgs.
func runTypeIDGenerator(t *testing.T, msgs ...*descriptorpb.DescriptorProto) *pluginpb.CodeGeneratorResponse {
	// Build the request protoc sends for one proto3 file.
	file := &descriptorpb.FileDescriptorProto{
		Name:        proto.String("a/b-ops.proto"),
		Package:     proto.String("a"),
		Syntax:      proto.String("proto3"),
		Options:     &descriptorpb.FileOptions{GoPackage: proto.String("example.com/a;a")},
		MessageType: msgs,
	}
	req := &pluginpb.CodeGeneratorRequest{
		FileToGenerate: []string{file.GetName()},
		ProtoFile:      []*descriptorpb.FileDescriptorProto{file},
	}
	input, err := proto.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}

	// Decode the generator's response.
	output, err := GenerateTypeIDs(input)
	if err != nil {
		t.Fatal(err)
	}
	resp := &pluginpb.CodeGeneratorResponse{}
	if err := proto.Unmarshal(output, resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestGenerateTypeIDs(t *testing.T) {
	// One file declares a typed message, a typed nested message and a plain one.
	resp := runTypeIDGenerator(t,
		typedMessageProto("Outer", "a/outer", typedMessageProto("Inner", "a/inner")),
		typedMessageProto("Plain", ""),
	)
	if resp.GetError() != "" {
		t.Fatal(resp.GetError())
	}
	if len(resp.GetFile()) != 1 {
		t.Fatalf("expected one file, got %d", len(resp.GetFile()))
	}

	// The file declares each typed message's id and method and lists both.
	out := resp.GetFile()[0]
	if out.GetName() != "example.com/a/b-ops_typeid.pb.go" {
		t.Fatalf("unexpected file name %q", out.GetName())
	}
	content := out.GetContent()
	for _, want := range []string{
		`const OuterTypeID = "a/outer"`,
		`const Outer_InnerTypeID = "a/inner"`,
		"func (*Outer) TypeID() string {",
		"var BOpsTypes = []typeid.Type{",
		"{ID: Outer_InnerTypeID, New: func() any { return new(Outer_Inner) }},",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in:\n%s", want, content)
		}
	}
	if strings.Contains(content, "Plain") {
		t.Errorf("untyped message generated:\n%s", content)
	}
}

func TestGenerateTypeIDsSkipsUntypedFile(t *testing.T) {
	// A file without typed messages produces no output.
	resp := runTypeIDGenerator(t, typedMessageProto("Plain", ""))
	if resp.GetError() != "" || len(resp.GetFile()) != 0 {
		t.Fatalf("expected no output, got error %q and %d files", resp.GetError(), len(resp.GetFile()))
	}
}

func TestGenerateTypeIDsRejectsDuplicate(t *testing.T) {
	// Two messages may not share a type id.
	resp := runTypeIDGenerator(t,
		typedMessageProto("First", "a/same"),
		typedMessageProto("Second", "a/same"),
	)
	if !strings.Contains(resp.GetError(), `type id "a/same" already declared by a.First`) {
		t.Fatalf("unexpected error %q", resp.GetError())
	}
}
