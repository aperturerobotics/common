package cases

import "contracts"

// emptyResult is safe because every return is proved nil in the imported package.
func emptyResult() {
	_ = contracts.Empty()
}

// inferredError uses the callee's proved error contract.
func inferredError(fail bool) error {
	x, err := contracts.Forward(fail)
	if err != nil {
		return err
	}
	defer x.Release()
	return nil
}

// inferredFound follows the same resource through an absence result.
func inferredFound(found bool) error {
	x, ok, err := contracts.Lookup(found)
	if err != nil || !ok {
		return err
	}
	x.Release()
	return nil
}

// inferredPartial still requires release of a partial result on error.
func inferredPartial() error {
	x, err := contracts.Partial() // want "x .* not released or transferred on all paths"
	if err != nil {
		return err
	}
	x.Release()
	return nil
}

// deferredResult cannot use a return expression that a defer later replaces.
func deferredResult() {
	_, _ = contracts.Deferred() // want "handles.Handle result .* is discarded"
}
