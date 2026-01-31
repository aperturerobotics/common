module github.com/aperturerobotics/common

go 1.25

// This fork uses protobuf-go-lite. This replace can be safely removed but optimizes binary size.
replace github.com/libp2p/go-libp2p => github.com/aperturerobotics/go-libp2p v0.37.1-0.20241111002741-5cfbb50b74e0 // aperture

replace github.com/ipfs/go-log/v2 => github.com/paralin/ipfs-go-logrus v0.0.0-20240410105224-e24cb05f9e98 // master

require (
	github.com/aperturerobotics/abseil-cpp v0.0.0-20260131110040-4bb56e2f9017 // aperture-2
	github.com/aperturerobotics/cli v1.1.0
	github.com/aperturerobotics/go-protoc-wasi v0.0.0-20260131050911-b5f94b044584
	github.com/aperturerobotics/protobuf v0.0.0-20260131033322-bd4a2148b9c4 // wasi
	github.com/aperturerobotics/protobuf-go-lite v0.12.0 // latest
)

require github.com/aperturerobotics/json-iterator-lite v1.0.1-0.20240713111131-be6bf89c3008 // indirect

require (
	github.com/tetratelabs/wazero v1.11.0
	github.com/xrash/smetrics v0.0.0-20250705151800-55b8f293f342 // indirect
	golang.org/x/mod v0.32.0
	golang.org/x/sys v0.38.0 // indirect
)
