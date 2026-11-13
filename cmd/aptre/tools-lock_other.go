//go:build !unix

package main

// lockToolsDir takes no lock where flock is missing, such as Windows, so
// checkouts there should keep a tools directory each, the default .tools.
func lockToolsDir(string) (func(), error) {
	return func() {}, nil
}
