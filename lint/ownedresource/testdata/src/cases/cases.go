package cases

import (
	"testing"

	h "handles"
)

// dropped covers result positions, initializers, var specs, and ignored calls.
func dropped() {
	_ = h.New()                            // want "owned handles.Handle result .* is discarded"
	h.New()                                // want "owned handles.Handle result .* is discarded"
	_, _, _ = h.Many()                     // want "owned handles.Handle result .* is discarded"
	var _, _, _ = h.Many()                 // want "owned handles.Handle result .* is discarded"
	if _, _, err := h.Many(); err != nil { // want "owned handles.Handle result .* is discarded"
		return
	}
	_, _ = h.Generic[int]() // want "owned handles.Handle result .* is discarded"
	_ = h.Ptr()             // want "owned \\*handles.Pointer result .* is discarded"
	defer h.New()           // want "owned handles.Handle result .* is discarded"
	go h.New()              // want "owned handles.Handle result .* is discarded"
	_, _ = 1, h.New()       // want "owned handles.Handle result .* is discarded"
	h.Other()
	h.Borrow()
}

// missing proves that reading a handle does not release it.
func missing() {
	x := h.New() // want "owned x .* not released or transferred on all paths"
	x.Read()
}

// blankUse does not transfer ownership.
func blankUse() {
	x := h.New() // want "owned x .* not released or transferred on all paths"
	_ = x
}

// branch leaks only along its early return.
func branch(stop bool) {
	x := h.New() // want "owned x .* not released or transferred on all paths"
	if stop {
		return
	}
	x.Release()
}

// both releases on each branch.
func both(stop bool) {
	x := h.New()
	if stop {
		x.Release()
		return
	}
	h.Release(x)
}

// deferred captures the original receiver before its local name changes.
func deferred() {
	x := h.New()
	defer x.Release()
	x = h.New()
	x.Release()
}

// alias follows the resource through local copies.
func alias() {
	x := h.New()
	y := x
	x = nil
	y.Release()
}

// droppedAlias must still release the copied handle.
func droppedAlias() {
	x := h.New() // want "owned x .* not released or transferred on all paths"
	y := x
	_ = y
}

// overwritten loses the old acquisition before the replacement is released.
func overwritten() {
	x := h.New() // want "owned x .* not released or transferred on all paths"
	x = h.New()
	x.Release()
}

// shadow keeps identifiers in nested scopes separate.
func shadow() {
	x := h.New() // want "owned x .* not released or transferred on all paths"
	if x := h.New(); x != nil {
		x.Release()
	}
	x.Read()
}

// returned transfers an acquired value to the caller.
func returned() h.Handle {
	x := h.New()
	return x
}

// named transfers a named result on a bare return.
func named() (x h.Handle) {
	x = h.New()
	return
}

// stored transfers a handle into a field.
func stored(dst *struct{ Handle h.Handle }) {
	x := h.New()
	dst.Handle = x
}

// wrapped transfers a handle into an aggregate returned to the caller.
func wrapped() any {
	x := h.New()
	return struct{ Handle h.Handle }{x}
}

// consumer uses the configured argument position.
func consumer() {
	x := h.New() // want "owned x .* not released or transferred on all paths"
	y := h.New()
	h.Consume(x, y)
}

// inspected passes a borrowed handle to an ordinary function.
func inspected() {
	x := h.New() // want "owned x .* not released or transferred on all paths"
	h.Inspect(x)
}

// methods covers method expressions for cleanup and explicit consumers.
func methods() {
	x := h.New()
	h.Handle.Release(x)
	y := h.New()
	h.Sink.Take(h.Sink{}, y)
}

// nilGuard excludes the path where no resource was acquired.
func nilGuard() {
	x := h.New()
	if x == nil {
		return
	}
	x.Release()
}

// errorGuard requires a declared guarantee about error results.
func errorGuard() {
	x, err := h.Fallible()
	if err != nil {
		return
	}
	x.Release()
}

// partialError must release a possible partial result.
func partialError() {
	x, err := h.Partial() // want "owned x .* not released or transferred on all paths"
	if err != nil {
		return
	}
	x.Release()
}

// changedError cannot reuse the acquisition's error guarantee.
func changedError(err error) {
	x, next := h.Fallible() // want "owned x .* not released or transferred on all paths"
	next = err
	if next != nil {
		return
	}
	x.Release()
}

// loop releases every acquired iteration.
func loop(n int) {
	for range n {
		x := h.New()
		x.Release()
	}
}

// loopLeak loses a value on the continue path, including the last iteration.
func loopLeak(n int, stop bool) {
	for range n {
		x := h.New() // want "owned x .* not released or transferred on all paths"
		if stop {
			continue
		}
		x.Release()
	}
}

// unusedClosure does not execute its cleanup.
func unusedClosure() {
	x := h.New() // want "owned x .* not released or transferred on all paths"
	cleanup := func() { x.Release() }
	_ = cleanup
}

// unusedMethodValue does not invoke or register the captured release method.
func unusedMethodValue() {
	x := h.New() // want "owned x .* not released or transferred on all paths"
	cleanup := x.Release
	_ = cleanup
}

// joinedAliases keeps distinct ownership states when branches rejoin.
func joinedAliases(keep bool) {
	x := h.New() // want "owned x .* not released or transferred on all paths"
	var alias h.Handle
	if keep {
		alias = x
	}
	if alias != nil {
		alias.Release()
	}
}

// closureDefer releases at function return.
func closureDefer() {
	x := h.New()
	defer func() { x.Release() }()
}

// closureOverwrite reads the replaced variable when deferred cleanup executes.
func closureOverwrite() {
	x := h.New() // want "owned x .* not released or transferred on all paths"
	defer func() { x.Release() }()
	x = h.New()
	x.Release()
}

// deferOrder loses the handle when the last registered defer clears it first.
func deferOrder() {
	x := h.New() // want "owned x .* not released or transferred on all paths"
	defer func() { x.Release() }()
	defer func() { x = nil }()
}

// deferReleaseFirst releases before the earlier registered defer clears it.
func deferReleaseFirst() {
	x := h.New()
	defer func() { x = nil }()
	defer func() { x.Release() }()
}

// conditionalClosure leaves an unreleased return path inside its defer.
func conditionalClosure(stop bool) {
	x := h.New() // want "owned x .* not released or transferred on all paths"
	defer func() {
		if stop {
			return
		}
		x.Release()
	}()
}

// invokedClosure executes cleanup immediately.
func invokedClosure() {
	x := h.New()
	func() { x.Release() }()
}

// testCleanup registers cleanup through the standard testing API.
func testCleanup(t *testing.T) {
	x := h.New()
	t.Cleanup(x.Release)
	y := h.New()
	t.Cleanup(func() { y.Release() })
}

// noReturn follows ctrlflow's panic termination rule.
func noReturn() {
	x := h.New()
	x.Read()
	panic("stop")
}

// unreachable ignores acquisitions after a nonreturning call.
func unreachable() {
	panic("stop")
	x := h.New()
	x.Read()
}

// variableConstructor uses the declared result type of a function value.
func variableConstructor(newHandle func() h.Handle) {
	x := newHandle() // want "owned x .* not released or transferred on all paths"
	x.Read()
}

// aliases preserve exact identities while conversions create no acquisition.
func aliases() {
	var x h.Alias = h.New()
	y := h.Handle(x)
	y.Release()
	p := h.Ptr()
	p.Close()
}
