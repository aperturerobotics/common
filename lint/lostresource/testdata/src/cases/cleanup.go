package cases

import h "handles"

// returnedCleanup transfers the obligation in a cleanup callback.
func returnedCleanup() func() {
	x := h.New()
	cleanup := func() { x.Release() }
	return cleanup
}

// returnedLiteral transfers an immediately returned callback.
func returnedLiteral() func() {
	x := h.New()
	return func() { x.Release() }
}

// returnedConditionalCleanup cannot promise to release the handle.
func returnedConditionalCleanup(stop bool) func() {
	x := h.New() // want "x .* not released or transferred on all paths"
	return func() {
		if stop {
			return
		}
		x.Release()
	}
}

// boundCleanup retains its receiver after the original variable changes.
func boundCleanup() func() {
	x := h.New()
	cleanup := x.Release
	x = nil
	return cleanup
}

// lostCleanup still leaks when an unused bound callback is discarded.
func lostCleanup() {
	x := h.New() // want "x .* not released or transferred on all paths"
	cleanup := x.Release
	x = nil
	_ = cleanup
}

// deferredLocal keeps late captured-variable semantics.
func deferredLocal() {
	x := h.New() // want "x .* not released or transferred on all paths"
	cleanup := func() { x.Release() }
	defer cleanup()
	x = nil
}

// adoption releases on error and transfers on success.
func adoption() error {
	x := h.New()
	_, err := h.Adopt(x)
	if err != nil {
		x.Release()
		return err
	}
	return nil
}

// adoptionError still requires the original handle's release.
func adoptionError() error {
	x := h.New() // want "x .* not released or transferred on all paths"
	_, err := h.Adopt(x)
	if err != nil {
		return err
	}
	return nil
}

// ignoredAdoptionError cannot establish a successful transfer.
func ignoredAdoptionError() {
	x := h.New() // want "x .* not released or transferred on all paths"
	_, _ = h.Adopt(x)
}

// cleanupList transfers bound release methods into a cleanup collection.
func cleanupList() []func() {
	x := h.New()
	var cleanups []func()
	cleanups = append(cleanups, x.Release)
	return cleanups
}

// cleanupLiteral transfers a closure inside an aggregate.
func cleanupLiteral() []func() {
	x := h.New()
	return []func(){func() { x.Release() }}
}

// readList still borrows the handle when recording a method that does not release it.
func readList() []func() int {
	x := h.New() // want "x .* not released or transferred on all paths"
	return []func() int{x.Read}
}

// callbackConsumer transfers cleanup only at an explicit ownership boundary.
func callbackConsumer() {
	x := h.New()
	h.TakeCleanup(func() { x.Release() })
}

// borrowingCallback cannot transfer an obligation it does not discharge.
func borrowingCallback() {
	x := h.New() // want "x .* not released or transferred on all paths"
	h.TakeCleanup(func() { x.Read() })
}
