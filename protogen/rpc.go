package protogen

import (
	"strings"

	"github.com/pkg/errors"
)

// RPCLibrary identifies an RPC stub generator family.
type RPCLibrary string

const (
	// RPCLibraryStarpc enables StarPC RPC stubs.
	RPCLibraryStarpc RPCLibrary = "starpc"
)

// RPCLibraries contains the enabled RPC stub generators.
type RPCLibraries map[RPCLibrary]struct{}

// NewRPCLibraries validates and normalizes RPC generator names.
func NewRPCLibraries(names []string) (RPCLibraries, error) {
	if len(names) == 0 {
		return RPCLibraries{RPCLibraryStarpc: {}}, nil
	}

	libs := make(RPCLibraries)
	for _, raw := range names {
		for _, field := range strings.Split(raw, ",") {
			name := strings.TrimSpace(field)
			if name == "" {
				continue
			}
			switch RPCLibrary(name) {
			case "none", "false":
				return RPCLibraries{}, nil
			case RPCLibraryStarpc:
				libs[RPCLibraryStarpc] = struct{}{}
			default:
				return nil, errors.Errorf("unknown RPC library %q", name)
			}
		}
	}
	return libs, nil
}

// Has returns true when the RPC generator is enabled.
func (r RPCLibraries) Has(lib RPCLibrary) bool {
	_, ok := r[lib]
	return ok
}
