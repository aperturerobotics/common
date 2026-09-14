package discard

import "handles"

// Check exercises the optional discard-only rollout mode.
func Check() {
	_ = handles.New() // want "owned handles.Handle result .* is discarded"
	h := handles.New()
	h.Read()
}
