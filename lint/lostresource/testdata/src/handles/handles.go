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

// Lookalike has the same methods but no configured resource contract.
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

// Sink accepts a handle through an explicitly configured method.
type Sink struct{}

// Take consumes its first argument.
func (Sink) Take(Handle) {}

// New acquires a handle.
func New() Handle

// Many returns its handle at a nonzero tuple position.
func Many() (int, Alias, error)

// Generic exercises generic call resolution.
func Generic[T any]() (T, Handle) { var t T; return t, New() }

// Ptr acquires a pointer through an alias.
func Ptr() PointerAlias

// Other returns an unconfigured structural lookalike.
func Other() Lookalike

// Borrow returns a handle held by another component.
func Borrow() Handle

// Partial may acquire a handle even on error.
func Partial() (Strict, error)

// Fallible acquires a handle only on success.
func Fallible() (Success, error)

// Adopt consumes a handle on success and leaves it to the caller on error.
func Adopt(Handle) (int, error)

// TakeCleanup takes responsibility for invoking the cleanup callback.
func TakeCleanup(func())

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
