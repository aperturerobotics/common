module github.com/aperturerobotics/common

go 1.24

// This fork uses protobuf-go-lite. This replace can be safely removed but optimizes binary size.
replace github.com/libp2p/go-libp2p => github.com/aperturerobotics/go-libp2p v0.37.1-0.20241111002741-5cfbb50b74e0 // aperture

replace github.com/ipfs/go-log/v2 => github.com/paralin/ipfs-go-logrus v0.0.0-20240410105224-e24cb05f9e98 // master

require github.com/aperturerobotics/protobuf-go-lite v0.8.0 // latest

require github.com/aperturerobotics/json-iterator-lite v1.0.1-0.20240713111131-be6bf89c3008 // indirect
