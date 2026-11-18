package protogen

import (
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

// The in-process generators read these custom option field numbers. The range
// 51700-51799 is reserved for Aperture options; take the next free number for
// a new one and list it here.
const (
	// typeIDOptionField is the (typeid.id) message option: the stable type
	// identifier string declared in typeid/typeid.proto.
	typeIDOptionField protowire.Number = 51700
	// worldRefOptionField is Spacewave's (world.ref) field option: a message
	// holding the reference predicate (1, string) and index flag (2, bool).
	worldRefOptionField protowire.Number = 51701
	// worldSpanOptionField is Spacewave's (world.span) message option: a
	// message holding the start (1, string) and end (2, string) field names.
	worldSpanOptionField protowire.Number = 51702
)

// scanOption calls cb with the value of each length-delimited occurrence of
// option field num in opts, in wire order.
//
// The option is read from the encoded options, so a generator needs no
// registered extension and reads it whether protoc's descriptor resolved the
// extension or left it as an unknown field.
// A string option takes its last occurrence.
// A message option merges its occurrences, as decoding them in order does.
func scanOption(opts proto.Message, num protowire.Number, cb func(v []byte) error) error {
	// Encode the options as they appear on the wire.
	b, err := proto.Marshal(opts)
	if err != nil {
		return err
	}

	// Pass each occurrence of the option field and skip all other fields.
	for len(b) != 0 {
		fnum, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return protowire.ParseError(n)
		}
		b = b[n:]
		if fnum == num && typ == protowire.BytesType {
			v, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return protowire.ParseError(n)
			}
			if err := cb(v); err != nil {
				return err
			}
			b = b[n:]
			continue
		}
		n = protowire.ConsumeFieldValue(fnum, typ, b)
		if n < 0 {
			return protowire.ParseError(n)
		}
		b = b[n:]
	}
	return nil
}

// scanOptionFields calls cb with each field of the encoded option message v.
// cb receives the field number, wire type and the remaining bytes, and returns
// how many bytes the field's value used, or a negative protowire error code.
func scanOptionFields(v []byte, cb func(num protowire.Number, typ protowire.Type, b []byte) int) error {
	for len(v) != 0 {
		num, typ, n := protowire.ConsumeTag(v)
		if n < 0 {
			return protowire.ParseError(n)
		}
		v = v[n:]
		n = cb(num, typ, v)
		if n < 0 {
			return protowire.ParseError(n)
		}
		v = v[n:]
	}
	return nil
}
