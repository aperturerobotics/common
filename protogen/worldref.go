package protogen

import (
	"strconv"

	liteprotogen "github.com/aperturerobotics/protobuf-go-lite/compiler/protogen"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

// WorldRefPluginName is the protoc plugin executable name of the Go World
// reference generator served in process.
const WorldRefPluginName = "protoc-gen-go-worldref"

// timestampName is the full name of the well-known Timestamp message.
const timestampName protoreflect.FullName = "google.protobuf.Timestamp"

// timePackage is the Go package of time.Time.
const timePackage = liteprotogen.GoImportPath("time")

// fmtPackage is the Go package of fmt.Errorf.
const fmtPackage = liteprotogen.GoImportPath("fmt")

// protowirePackage is the lite runtime's wire format package.
const protowirePackage = liteprotogen.GoImportPath("github.com/aperturerobotics/protobuf-go-lite/encoding/protowire")

// worldRef is a decoded (world.ref) field option.
type worldRef struct {
	// predicate is the graph predicate of the reference.
	predicate string
	// index is set when the reference is written as a graph quad.
	index bool
}

// worldSpan is a decoded (world.span) message option, resolved to its fields.
type worldSpan struct {
	// start is the field holding the start time.
	start protoreflect.FieldDescriptor
	// end is the field holding the end time, or nil if none is declared.
	end protoreflect.FieldDescriptor
}

// worldRefIndex records which message types carry World references, directly
// through a (world.ref) field or through a message field whose type does.
type worldRefIndex map[protoreflect.FullName]bool

// GenerateWorldRefs answers one encoded CodeGeneratorRequest for the Go World
// reference generator. For each proto file with a message that carries a
// (world.ref) field or declares (world.span) it emits <name>_worldref.pb.go
// holding the message's RangeWorldRefs, WorldSpan and ScanWorldSpan methods.
func GenerateWorldRefs(input []byte) ([]byte, error) {
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

	// Index every message of the request, imports included, so a field typed
	// by another file's message knows whether that message carries references.
	index, err := buildWorldRefIndex(gen.Files)
	if err != nil {
		gen.Error(err)
		return proto.Marshal(gen.Response())
	}

	// Emit one file per requested proto that has a message to extend.
	for _, file := range gen.Files {
		if !file.Generate {
			continue
		}
		if err := generateWorldRefFile(gen, file, index); err != nil {
			gen.Error(err)
			break
		}
	}

	return proto.Marshal(gen.Response())
}

// buildWorldRefIndex marks the messages of files that carry World references.
func buildWorldRefIndex(files []*liteprotogen.File) (worldRefIndex, error) {
	// Mark each message that declares a reference field of its own.
	index := make(worldRefIndex)
	var msgs []protoreflect.MessageDescriptor
	for _, file := range files {
		var err error
		msgs, err = appendWorldRefMessages(msgs, file.Desc.Messages(), index)
		if err != nil {
			return nil, err
		}
	}

	// Propagate the mark to each message holding a marked message, until no
	// mark changes. Cycles through message fields settle like any other path.
	for changed := true; changed; {
		changed = false
		for _, md := range msgs {
			if !index[md.FullName()] && index.holdsCarrier(md) {
				index[md.FullName()] = true
				changed = true
			}
		}
	}
	return index, nil
}

// appendWorldRefMessages appends msgs and their nested messages to all and
// marks each one that declares a valid (world.ref) field.
func appendWorldRefMessages(all []protoreflect.MessageDescriptor, msgs protoreflect.MessageDescriptors, index worldRefIndex) ([]protoreflect.MessageDescriptor, error) {
	for i := range msgs.Len() {
		// Read each field's option, rejecting a misplaced one.
		md := msgs.Get(i)
		fields := md.Fields()
		for j := range fields.Len() {
			_, ok, err := fieldWorldRef(fields.Get(j))
			if err != nil {
				return nil, errors.Wrap(err, string(fields.Get(j).FullName()))
			}
			if ok {
				index[md.FullName()] = true
			}
		}

		// Nested messages follow their parent.
		all = append(all, md)
		var err error
		all, err = appendWorldRefMessages(all, md.Messages(), index)
		if err != nil {
			return nil, err
		}
	}
	return all, nil
}

// holdsCarrier reports whether md has a message, repeated message or map
// value field whose type carries references.
func (x worldRefIndex) holdsCarrier(md protoreflect.MessageDescriptor) bool {
	fields := md.Fields()
	for i := range fields.Len() {
		if sub := fieldMessage(fields.Get(i)); sub != nil && x[sub.FullName()] {
			return true
		}
	}
	return false
}

// fieldMessage returns the message type a field holds: its own message type,
// or a map's value type. Returns nil for scalar fields and scalar maps.
func fieldMessage(fd protoreflect.FieldDescriptor) protoreflect.MessageDescriptor {
	if fd.IsMap() {
		return fd.MapValue().Message()
	}
	return fd.Message()
}

// fieldWorldRef reads the (world.ref) option of a field.
// The option is valid only on a string or repeated string field.
func fieldWorldRef(fd protoreflect.FieldDescriptor) (worldRef, bool, error) {
	// Merge each occurrence of the option message.
	var ref worldRef
	var found bool
	err := scanOption(fd.Options(), worldRefOptionField, func(v []byte) error {
		found = true
		return scanOptionFields(v, func(num protowire.Number, typ protowire.Type, b []byte) int {
			switch {
			case num == 1 && typ == protowire.BytesType:
				v, n := protowire.ConsumeBytes(b)
				ref.predicate = string(v)
				return n
			case num == 2 && typ == protowire.VarintType:
				v, n := protowire.ConsumeVarint(b)
				ref.index = v != 0
				return n
			default:
				return protowire.ConsumeFieldValue(num, typ, b)
			}
		})
	})
	if err != nil || !found {
		return worldRef{}, false, err
	}

	// A reference names an object key and its predicate.
	if fd.Kind() != protoreflect.StringKind || fd.IsMap() {
		return worldRef{}, false, errors.New("world.ref requires a string or repeated string field")
	}
	if ref.predicate == "" {
		return worldRef{}, false, errors.New("world.ref predicate is empty")
	}
	return ref, true, nil
}

// messageWorldSpan reads and resolves the (world.span) option of a message.
// Each named field must be a singular string holding an RFC 3339 time or a
// google.protobuf.Timestamp. The end field is optional.
func messageWorldSpan(md protoreflect.MessageDescriptor) (worldSpan, bool, error) {
	// Merge each occurrence of the option message.
	var start, end string
	var found bool
	err := scanOption(md.Options(), worldSpanOptionField, func(v []byte) error {
		found = true
		return scanOptionFields(v, func(num protowire.Number, typ protowire.Type, b []byte) int {
			if typ != protowire.BytesType || (num != 1 && num != 2) {
				return protowire.ConsumeFieldValue(num, typ, b)
			}
			v, n := protowire.ConsumeBytes(b)
			if num == 1 {
				start = string(v)
			} else {
				end = string(v)
			}
			return n
		})
	})
	if err != nil || !found {
		return worldSpan{}, false, err
	}

	// Resolve the declared names to time fields.
	if start == "" {
		return worldSpan{}, false, errors.New("world.span start is empty")
	}
	var span worldSpan
	if span.start, err = spanField(md, start); err != nil {
		return worldSpan{}, false, err
	}
	if end != "" {
		if span.end, err = spanField(md, end); err != nil {
			return worldSpan{}, false, err
		}
	}
	return span, true, nil
}

// spanField looks up the time field name of md.
func spanField(md protoreflect.MessageDescriptor, name string) (protoreflect.FieldDescriptor, error) {
	// Require the named field to exist.
	fd := md.Fields().ByName(protoreflect.Name(name))
	if fd == nil {
		return nil, errors.Errorf("world.span field %q not found", name)
	}

	// Require a singular time field.
	isTimestamp := fd.Message() != nil && fd.Message().FullName() == timestampName
	if fd.IsList() || fd.IsMap() || (fd.Kind() != protoreflect.StringKind && !isTimestamp) {
		return nil, errors.Errorf("world.span field %q must be a singular string or google.protobuf.Timestamp", name)
	}
	return fd, nil
}

// generateWorldRefFile writes the reference and span methods of one file's
// messages, if any message needs one.
func generateWorldRefFile(gen *liteprotogen.Plugin, file *liteprotogen.File, index worldRefIndex) error {
	var g *liteprotogen.GeneratedFile
	var walk func(msgs []*liteprotogen.Message) error
	walk = func(msgs []*liteprotogen.Message) error {
		for _, msg := range msgs {
			// Skip map entries: Go has no type for them, and the map's owner
			// walks their values.
			if msg.Desc.IsMapEntry() {
				continue
			}

			// Read the message's span before emitting anything for it.
			span, hasSpan, err := messageWorldSpan(msg.Desc)
			if err != nil {
				return errors.Wrap(err, string(msg.Desc.FullName()))
			}
			carries := index[msg.Desc.FullName()]

			// Open the output on the file's first extended message.
			if (carries || hasSpan) && g == nil {
				g = gen.NewGeneratedFile(file.GeneratedFilenamePrefix+"_worldref.pb.go", file.GoImportPath)
				g.P("// Code generated by ", WorldRefPluginName, ". DO NOT EDIT.")
				g.P("// source: ", file.Desc.Path())
				g.P()
				g.P("package ", file.GoPackageName)
				g.P()
			}

			// Emit the message's methods, then visit its nested messages.
			if carries {
				generateRangeWorldRefs(g, msg, index)
			}
			if hasSpan {
				generateWorldSpan(g, msg, span)
				generateScanWorldSpan(g, msg, span)
			}
			if err := walk(msg.Messages); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(file.Messages)
}

// generateRangeWorldRefs writes the RangeWorldRefs method of msg, which walks
// its reference fields and the references of its message fields in field
// order. Map values are walked in Go's map order, which is random.
func generateRangeWorldRefs(g *liteprotogen.GeneratedFile, msg *liteprotogen.Message, index worldRefIndex) {
	// Name each predicate the message declares, so readers of its edges share
	// the declared string.
	name := msg.GoIdent.GoName
	generatePredicateConsts(g, msg)

	// Declare the method.
	g.P("// RangeWorldRefs calls cb with each World reference ", name, " declares,")
	g.P("// including those of its message fields, until cb returns false.")
	g.P("// Empty references are skipped.")
	g.P("//")
	g.P("// Returns false if cb stopped the walk.")
	g.P("func (m *", name, ") RangeWorldRefs(cb func(predicate, target string, indexed bool) bool) bool {")

	// Pass each reference and walk each message holding references.
	for _, f := range msg.Fields {
		getter := "m.Get" + f.GoName + "()"
		if ref, ok, _ := fieldWorldRef(f.Desc); ok {
			call := "!cb(" + predicateConst(msg, f) + ", v, " + strconv.FormatBool(ref.index) + ")"
			if f.Desc.IsList() {
				g.P("for _, v := range ", getter, " {")
				g.P("if v != \"\" && ", call, " {")
				g.P("return false")
				g.P("}")
				g.P("}")
			} else {
				g.P("if v := ", getter, "; v != \"\" && ", call, " {")
				g.P("return false")
				g.P("}")
			}
			continue
		}
		sub := fieldMessage(f.Desc)
		if sub == nil || !index[sub.FullName()] {
			continue
		}
		if f.Desc.IsList() || f.Desc.IsMap() {
			g.P("for _, v := range ", getter, " {")
			g.P("if !v.RangeWorldRefs(cb) {")
			g.P("return false")
			g.P("}")
			g.P("}")
		} else {
			g.P("if !", getter, ".RangeWorldRefs(cb) {")
			g.P("return false")
			g.P("}")
		}
	}
	g.P("return true")
	g.P("}")
	g.P()
}

// generatePredicateConsts writes one constant per reference field of msg,
// holding the predicate its world.ref option declares.
func generatePredicateConsts(g *liteprotogen.GeneratedFile, msg *liteprotogen.Message) {
	// Collect the declared predicates in field order.
	var refs []*liteprotogen.Field
	for _, f := range msg.Fields {
		if _, ok, _ := fieldWorldRef(f.Desc); ok {
			refs = append(refs, f)
		}
	}
	if len(refs) == 0 {
		return
	}

	// Emit them as one constant block.
	g.P("const (")
	for _, f := range refs {
		ref, _, _ := fieldWorldRef(f.Desc)
		g.P("// ", predicateConst(msg, f), " is the World predicate of ", msg.GoIdent.GoName, ".", f.GoName, ".")
		g.P(predicateConst(msg, f), " = ", strconv.Quote(ref.predicate))
	}
	g.P(")")
	g.P()
}

// predicateConst returns the Go name of the predicate constant of field f.
func predicateConst(msg *liteprotogen.Message, f *liteprotogen.Field) string {
	return msg.GoIdent.GoName + "_" + f.GoName + "Predicate"
}

// generateWorldSpan writes the WorldSpan method of msg.
func generateWorldSpan(g *liteprotogen.GeneratedFile, msg *liteprotogen.Message, span worldSpan) {
	// Declare the method.
	name := msg.GoIdent.GoName
	timeType := g.QualifiedGoIdent(timePackage.Ident("Time"))
	g.P("// WorldSpan returns the start and end times ", name, " declares with world.span.")
	g.P("// An unset time is zero.")
	g.P("// Returns an error if a string time is not RFC 3339.")
	g.P("func (m *", name, ") WorldSpan() (start, end ", timeType, ", err error) {")

	// Read each declared time field.
	generateSpanTime(g, "start", goField(msg, span.start))
	if span.end != nil {
		generateSpanTime(g, "end", goField(msg, span.end))
	}
	g.P("return start, end, nil")
	g.P("}")
	g.P()
}

// generateScanWorldSpan writes the ScanWorldSpan method of msg, which reads
// the span from msg's wire encoding: it walks the top-level fields once, reads
// only the span fields, and skips the others without decoding them. A field
// that occurs more than once resolves as a full decode resolves it: a string
// takes its last value and a Timestamp merges its occurrences.
func generateScanWorldSpan(g *liteprotogen.GeneratedFile, msg *liteprotogen.Message, span worldSpan) {
	// Collect the span fields.
	fields := []spanScanField{{out: "start", field: goField(msg, span.start)}}
	if span.end != nil {
		fields = append(fields, spanScanField{out: "end", field: goField(msg, span.end)})
	}

	// Declare the method and the state of each span field.
	name := msg.GoIdent.GoName
	g.P("// ScanWorldSpan returns the span WorldSpan returns for the ", name, " encoded")
	g.P("// in data. It reads only the span fields and skips the others undecoded, so")
	g.P("// it allocates nothing for them.")
	g.P("// Returns an error if a span field is malformed or a string time is not")
	g.P("// RFC 3339.")
	g.P("func (*", name, ") ScanWorldSpan(data []byte) (start, end ", g.QualifiedGoIdent(timePackage.Ident("Time")), ", err error) {")
	for _, f := range fields {
		f.declare(g)
	}

	// Read each top-level tag.
	g.P("for len(data) > 0 {")
	g.P("num, typ, n := ", protowireIdent(g, "ConsumeTag"), "(data)")
	generateAdvance(g, "data", "n")

	// Read the span fields and skip the rest.
	g.P("switch num {")
	for _, f := range fields {
		f.scan(g)
	}
	g.P("default:")
	g.P("n = ", protowireIdent(g, "ConsumeFieldValue"), "(num, typ, data)")
	generateAdvance(g, "data", "n")
	g.P("}")
	g.P("}")

	// Convert each read field to its time.
	for _, f := range fields {
		f.convert(g)
	}
	g.P("return start, end, nil")
	g.P("}")
	g.P()
}

// generateAdvance writes the statements that return the parse error of a
// negative length n, or else advance buf past n bytes.
func generateAdvance(g *liteprotogen.GeneratedFile, buf, n string) {
	g.P("if ", n, " < 0 {")
	g.P("return start, end, ", protowireIdent(g, "ParseError"), "(", n, ")")
	g.P("}")
	g.P(buf, " = ", buf, "[", n, ":]")
}

// generateWireType writes the statement that rejects a wire type typ other
// than want, as a full decode of field rejects it.
func generateWireType(g *liteprotogen.GeneratedFile, typ, want, field string) {
	errorf := g.QualifiedGoIdent(fmtPackage.Ident("Errorf"))
	g.P("if ", typ, " != ", protowireIdent(g, want), " {")
	g.P("return start, end, ", errorf, "(\"proto: wrong wireType = %d for field ", field, "\", ", typ, ")")
	g.P("}")
}

// spanScanField is one span field ScanWorldSpan reads.
type spanScanField struct {
	// out is the result the field sets: start or end.
	out string
	// field is the time field.
	field *liteprotogen.Field
}

// isText reports whether the field holds an RFC 3339 string.
func (f spanScanField) isText() bool {
	return f.field.Desc.Kind() == protoreflect.StringKind
}

// declare writes the variables holding what the scan has read of the field.
func (f spanScanField) declare(g *liteprotogen.GeneratedFile) {
	if f.isText() {
		g.P("var ", f.out, "Text []byte")
		return
	}
	g.P("var ", f.out, "Set bool")
	g.P("var ", f.out, "Seconds, ", f.out, "Nanos int64")
}

// scan writes the switch case that reads one occurrence of the field.
func (f spanScanField) scan(g *liteprotogen.GeneratedFile) {
	// Read the length-delimited value; a string keeps its last value.
	g.P("case ", strconv.Itoa(int(f.field.Desc.Number())), ":")
	generateWireType(g, "typ", "BytesType", f.field.GoName)
	g.P("v, n := ", protowireIdent(g, "ConsumeBytes"), "(data)")
	generateAdvance(g, "data", "n")
	if f.isText() {
		g.P(f.out, "Text = v")
		return
	}

	// Read each tag of the Timestamp.
	g.P(f.out, "Set = true")
	g.P("for len(v) > 0 {")
	g.P("tnum, ttyp, tn := ", protowireIdent(g, "ConsumeTag"), "(v)")
	generateAdvance(g, "v", "tn")

	// Skip fields other than its seconds and nanos.
	g.P("if tnum != 1 && tnum != 2 {")
	g.P("tn = ", protowireIdent(g, "ConsumeFieldValue"), "(tnum, ttyp, v)")
	generateAdvance(g, "v", "tn")
	g.P("continue")
	g.P("}")

	// Read the seconds or nanos.
	generateWireType(g, "ttyp", "VarintType", f.field.GoName)
	g.P("x, tn := ", protowireIdent(g, "ConsumeVarint"), "(v)")
	generateAdvance(g, "v", "tn")

	// Merge it into what earlier occurrences set.
	g.P("if tnum == 1 {")
	g.P(f.out, "Seconds = int64(x)")
	g.P("} else {")
	g.P(f.out, "Nanos = int64(int32(x))")
	g.P("}")
	g.P("}")
}

// convert writes the statements that set the field's result from what the
// scan read, as WorldSpan sets it from a decoded message.
func (f spanScanField) convert(g *liteprotogen.GeneratedFile) {
	if f.isText() {
		parse := g.QualifiedGoIdent(timePackage.Ident("Parse"))
		layout := g.QualifiedGoIdent(timePackage.Ident("RFC3339Nano"))
		g.P("if len(", f.out, "Text) != 0 {")
		g.P("if ", f.out, ", err = ", parse, "(", layout, ", string(", f.out, "Text)); err != nil {")
		g.P("return start, end, err")
		g.P("}")
		g.P("}")
		return
	}
	g.P("if ", f.out, "Set {")
	g.P(f.out, " = ", g.QualifiedGoIdent(timePackage.Ident("Unix")), "(", f.out, "Seconds, ", f.out, "Nanos).UTC()")
	g.P("}")
}

// protowireIdent returns the qualified name of a lite protowire identifier.
func protowireIdent(g *liteprotogen.GeneratedFile, name string) string {
	return g.QualifiedGoIdent(protowirePackage.Ident(name))
}

// goField returns the generated field of msg described by fd.
func goField(msg *liteprotogen.Message, fd protoreflect.FieldDescriptor) *liteprotogen.Field {
	return msg.Fields[fd.Index()]
}

// generateSpanTime writes the statements that set the result named out from
// the time field f.
func generateSpanTime(g *liteprotogen.GeneratedFile, out string, f *liteprotogen.Field) {
	// Parse a string time as RFC 3339.
	getter := "m.Get" + f.GoName + "()"
	if f.Desc.Kind() == protoreflect.StringKind {
		g.P("if v := ", getter, "; v != \"\" {")
		g.P("if ", out, ", err = ", g.QualifiedGoIdent(timePackage.Ident("Parse")), "(", g.QualifiedGoIdent(timePackage.Ident("RFC3339Nano")), ", v); err != nil {")
		g.P("return start, end, err")
		g.P("}")
		g.P("}")
		return
	}

	// Convert a set Timestamp.
	g.P("if v := ", getter, "; v != nil {")
	g.P(out, " = v.AsTime()")
	g.P("}")
}
