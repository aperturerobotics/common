package protogen

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/pluginpb"
)

// refField builds a field descriptor with an optional (world.ref) option.
func refField(name string, num int32, label descriptorpb.FieldDescriptorProto_Label, typ descriptorpb.FieldDescriptorProto_Type, typeName, predicate string, index bool) *descriptorpb.FieldDescriptorProto {
	// Declare the field itself.
	fd := &descriptorpb.FieldDescriptorProto{
		Name:   proto.String(name),
		Number: proto.Int32(num),
		Label:  label.Enum(),
		Type:   typ.Enum(),
	}
	if typeName != "" {
		fd.TypeName = proto.String(typeName)
	}
	if predicate == "" {
		return fd
	}

	// Encode the option as protoc leaves an unregistered extension.
	var opt []byte
	opt = protowire.AppendTag(opt, 1, protowire.BytesType)
	opt = protowire.AppendString(opt, predicate)
	if index {
		opt = protowire.AppendTag(opt, 2, protowire.VarintType)
		opt = protowire.AppendVarint(opt, 1)
	}
	raw := protowire.AppendTag(nil, worldRefOptionField, protowire.BytesType)
	fd.Options = &descriptorpb.FieldOptions{}
	fd.Options.ProtoReflect().SetUnknown(protowire.AppendBytes(raw, opt))
	return fd
}

// spanOptions encodes a (world.span) option naming start and end.
func spanOptions(start, end string) *descriptorpb.MessageOptions {
	var opt []byte
	opt = protowire.AppendTag(opt, 1, protowire.BytesType)
	opt = protowire.AppendString(opt, start)
	opt = protowire.AppendTag(opt, 2, protowire.BytesType)
	opt = protowire.AppendString(opt, end)
	raw := protowire.AppendTag(nil, worldSpanOptionField, protowire.BytesType)
	opts := &descriptorpb.MessageOptions{}
	opts.ProtoReflect().SetUnknown(protowire.AppendBytes(raw, opt))
	return opts
}

// runWorldRefGenerator runs GenerateWorldRefs over one file holding msgs,
// which may use google.protobuf.Timestamp.
func runWorldRefGenerator(t *testing.T, msgs ...*descriptorpb.DescriptorProto) *pluginpb.CodeGeneratorResponse {
	// Build the request protoc sends for one proto3 file and its import.
	t.Helper()
	ts := protodesc.ToFileDescriptorProto(timestamppb.File_google_protobuf_timestamp_proto)
	file := &descriptorpb.FileDescriptorProto{
		Name:        proto.String("a/b-ops.proto"),
		Package:     proto.String("a"),
		Syntax:      proto.String("proto3"),
		Dependency:  []string{ts.GetName()},
		Options:     &descriptorpb.FileOptions{GoPackage: proto.String("example.com/a;a")},
		MessageType: msgs,
	}
	req := &pluginpb.CodeGeneratorRequest{
		FileToGenerate: []string{file.GetName()},
		ProtoFile:      []*descriptorpb.FileDescriptorProto{ts, file},
	}
	input, err := proto.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}

	// Decode the generator's response.
	output, err := GenerateWorldRefs(input)
	if err != nil {
		t.Fatal(err)
	}
	resp := &pluginpb.CodeGeneratorResponse{}
	if err := proto.Unmarshal(output, resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestGenerateWorldRefs(t *testing.T) {
	// Inner declares one reference; Record holds scalar, repeated, unindexed,
	// nested and map-valued references and a span; A reaches Inner only through a cycle
	// with B; Plain declares nothing.
	opt := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	rep := descriptorpb.FieldDescriptorProto_LABEL_REPEATED
	str := descriptorpb.FieldDescriptorProto_TYPE_STRING
	msg := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE
	inner := &descriptorpb.DescriptorProto{
		Name:  proto.String("Inner"),
		Field: []*descriptorpb.FieldDescriptorProto{refField("target", 1, opt, str, "", "a/inner", true)},
	}
	record := &descriptorpb.DescriptorProto{
		Name: proto.String("Record"),
		Field: []*descriptorpb.FieldDescriptorProto{
			refField("parent_key", 1, opt, str, "", "a/parent", true),
			refField("input_keys", 2, rep, str, "", "a/input", true),
			refField("note_key", 3, opt, str, "", "a/note", false),
			refField("inner", 4, opt, msg, ".a.Inner", "", false),
			refField("inners", 5, rep, msg, ".a.Inner", "", false),
			refField("created_at", 6, opt, str, "", "", false),
			refField("ended_at", 7, opt, msg, ".google.protobuf.Timestamp", "", false),
			refField("plain", 8, opt, str, "", "", false),
			refField("items", 9, rep, msg, ".a.Record.ItemsEntry", "", false),
		},
		NestedType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("ItemsEntry"),
			Field: []*descriptorpb.FieldDescriptorProto{
				refField("key", 1, opt, descriptorpb.FieldDescriptorProto_TYPE_INT32, "", "", false),
				refField("value", 2, opt, msg, ".a.Inner", "", false),
			},
			Options: &descriptorpb.MessageOptions{MapEntry: proto.Bool(true)},
		}},
		Options: spanOptions("created_at", "ended_at"),
	}
	a := &descriptorpb.DescriptorProto{
		Name: proto.String("A"),
		Field: []*descriptorpb.FieldDescriptorProto{
			refField("b", 1, opt, msg, ".a.B", "", false),
			refField("c", 2, opt, msg, ".a.Inner", "", false),
		},
	}
	b := &descriptorpb.DescriptorProto{
		Name:  proto.String("B"),
		Field: []*descriptorpb.FieldDescriptorProto{refField("a", 1, opt, msg, ".a.A", "", false)},
	}
	plain := &descriptorpb.DescriptorProto{
		Name:  proto.String("Plain"),
		Field: []*descriptorpb.FieldDescriptorProto{refField("key", 1, opt, str, "", "", false)},
	}
	resp := runWorldRefGenerator(t, inner, record, a, b, plain)
	if resp.GetError() != "" {
		t.Fatal(resp.GetError())
	}
	if len(resp.GetFile()) != 1 {
		t.Fatalf("expected one file, got %d", len(resp.GetFile()))
	}

	// The file walks every reference in field order and reads the span.
	out := resp.GetFile()[0]
	if out.GetName() != "example.com/a/b-ops_worldref.pb.go" {
		t.Fatalf("unexpected file name %q", out.GetName())
	}
	content := out.GetContent()
	for _, want := range []string{
		"func (m *Inner) RangeWorldRefs(cb func(predicate, target string, indexed bool) bool) bool {",
		`if v := m.GetTarget(); v != "" && !cb("a/inner", v, true) {`,
		`if v := m.GetParentKey(); v != "" && !cb("a/parent", v, true) {`,
		"for _, v := range m.GetInputKeys() {",
		`if v != "" && !cb("a/input", v, true) {`,
		`if v := m.GetNoteKey(); v != "" && !cb("a/note", v, false) {`,
		"if !m.GetInner().RangeWorldRefs(cb) {",
		"for _, v := range m.GetInners() {",
		"for _, v := range m.GetItems() {",
		"func (m *A) RangeWorldRefs(",
		"if !m.GetB().RangeWorldRefs(cb) {",
		"func (m *B) RangeWorldRefs(",
		"if !m.GetA().RangeWorldRefs(cb) {",
		"func (m *Record) WorldSpan() (start, end time.Time, err error) {",
		"if start, err = time.Parse(time.RFC3339Nano, v); err != nil {",
		"if v := m.GetEndedAt(); v != nil {",
		"end = v.AsTime()",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in:\n%s", want, content)
		}
	}
	for _, unwanted := range []string{"Plain", "GetPlain", "GetCreatedAt().RangeWorldRefs", "ItemsEntry"} {
		if strings.Contains(content, unwanted) {
			t.Errorf("unexpected %q in:\n%s", unwanted, content)
		}
	}
}

func TestGenerateWorldRefsSkipsPlainFile(t *testing.T) {
	// A file without references or spans produces no output.
	resp := runWorldRefGenerator(t, &descriptorpb.DescriptorProto{Name: proto.String("Plain")})
	if resp.GetError() != "" || len(resp.GetFile()) != 0 {
		t.Fatalf("expected no output, got error %q and %d files", resp.GetError(), len(resp.GetFile()))
	}
}

func TestGenerateWorldRefsRejectsNonString(t *testing.T) {
	// A reference must be held in a string field.
	field := refField("count", 1, descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL,
		descriptorpb.FieldDescriptorProto_TYPE_INT64, "", "a/count", true)
	resp := runWorldRefGenerator(t, &descriptorpb.DescriptorProto{
		Name:  proto.String("Bad"),
		Field: []*descriptorpb.FieldDescriptorProto{field},
	})
	if !strings.Contains(resp.GetError(), "a.Bad.count: world.ref requires a string or repeated string field") {
		t.Fatalf("unexpected error %q", resp.GetError())
	}
}
