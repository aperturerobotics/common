package handles

// Handle requires explicit cleanup; Read borrows the receiver.
type Handle interface {
	// Read returns a value without releasing the handle.
	Read() int
	// Release releases the handle.
	Release()
}

// Alias preserves the declared handle identity.
type Alias = Handle

// Lookalike has the same methods but no configured ownership contract.
type Lookalike interface {
	Read() int
	Release()
}

// Pointer exercises aliases of pointer results.
type Pointer struct{}

// PointerAlias preserves pointer identity.
type PointerAlias = *Pointer

// Close releases the pointer handle.
func (*Pointer) Close() {}

// Strict permits partially acquired handles on error.
type Strict interface{ Release() }

// Success guarantees no acquired handle when the error is non-nil.
type Success interface{ Release() }

// Sink accepts ownership through an explicitly configured method.
type Sink struct{}

// Take consumes its first argument.
func (Sink) Take(Handle) {}

// New acquires a handle.
func New() Handle { return nil }

// Many returns its owned handle at a nonzero tuple position.
func Many() (int, Alias, error) { return 0, nil, nil }

// Generic exercises generic call resolution.
func Generic[T any]() (T, Handle) { var t T; return t, nil }

// Ptr acquires a pointer through an alias.
func Ptr() PointerAlias { return nil }

// Other returns an unconfigured structural lookalike.
func Other() Lookalike { return nil }

// Borrow returns a handle owned by another component.
func Borrow() Handle { return nil }

// Partial may acquire a handle even on error.
func Partial() (Strict, error) { return nil, nil }

// Fallible acquires a handle only on success.
func Fallible() (Success, error) { return nil, nil }

// Release consumes its argument.
func Release(Handle) {}

// Consume borrows the first handle and consumes the second.
func Consume(Handle, Handle) {}

// Inspect borrows its argument.
func Inspect(Handle) {}

// Flag borrows its argument while returning a condition.
func Flag(Handle) bool { return true }

// ReleaseFlag consumes its argument while returning a condition.
func ReleaseFlag(h Handle) bool { h.Release(); return true }
