// Package typeid declares stable type identifiers for protobuf messages.
//
// A message declares its identifier with the (typeid.id) option. The aptre
// generator then emits a TypeID method for the message and lists the typed
// messages of each proto file as a []Type.
package typeid

// Type is a message type with a stable identifier.
type Type struct {
	// ID is the stable type identifier.
	ID string
	// New constructs an empty message of the type.
	New func() any
}
