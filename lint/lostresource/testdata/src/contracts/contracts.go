package contracts

import h "handles"

// failure supplies an error without making its implementation part of the fixture.
func failure() error

// Forward proves its result despite being declared before the implementation.
func Forward(fail bool) (h.Handle, error) { // want Forward:".*"
	return Acquire(fail)
}

// Empty never returns a handle despite its result type.
func Empty() h.Handle { // want Empty:".*"
	return nil
}

// Acquire returns no handle when acquisition fails.
func Acquire(fail bool) (h.Handle, error) { // want Acquire:".*"
	if fail {
		return nil, failure()
	}
	return h.New(), nil
}

// Lookup returns no handle when absent or failed.
func Lookup(found bool) (h.Handle, bool, error) { // want Lookup:".*"
	if !found {
		return nil, false, nil
	}
	return h.New(), true, nil
}

// Partial can return a handle and an error together.
func Partial() (h.Handle, error) {
	return h.New(), failure()
}

// Deferred changes named results after the explicit return expressions.
func Deferred() (out h.Handle, err error) {
	defer func() { out = h.New(); err = failure() }()
	return nil, nil
}
