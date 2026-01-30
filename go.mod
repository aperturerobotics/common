module github.com/aperturerobotics/common

go 1.25

// This fork uses protobuf-go-lite. This replace can be safely removed but optimizes binary size.
replace github.com/libp2p/go-libp2p => github.com/aperturerobotics/go-libp2p v0.37.1-0.20241111002741-5cfbb50b74e0 // aperture

replace github.com/ipfs/go-log/v2 => github.com/paralin/ipfs-go-logrus v0.0.0-20240410105224-e24cb05f9e98 // master

require (
	github.com/aperturerobotics/abseil-cpp v0.0.0-20260130220554-305ed0ea7006
	github.com/aperturerobotics/cli v1.1.0
	github.com/aperturerobotics/go-protoc-wasi v0.0.0-20260131033208-273d2014699f
	github.com/aperturerobotics/protobuf v0.0.0-20260131031545-7265127e58f9
	github.com/aperturerobotics/protobuf-go-lite v0.12.0 // latest
	github.com/tetratelabs/wazero v1.8.2
	golang.org/x/mod v0.22.0
)

require (
	github.com/aperturerobotics/json-iterator-lite v1.0.1-0.20240713111131-be6bf89c3008 // indirect
	github.com/xrash/smetrics v0.0.0-20250705151800-55b8f293f342 // indirect
)
