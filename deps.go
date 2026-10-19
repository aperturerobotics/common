//go:build deps_only

package main

import (
	// _ imports the starpc root to vendor its CMakeLists.txt.
	_ "github.com/aperturerobotics/starpc"
	// _ imports rpcstream to vendor its C++ sources.
	_ "github.com/aperturerobotics/starpc/rpcstream"
)
