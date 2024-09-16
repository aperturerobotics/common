module github.com/aperturerobotics/common

go 1.22

// This fork uses protobuf-go-lite. This replace can be safely removed but optimizes binary size.
replace github.com/libp2p/go-libp2p => github.com/aperturerobotics/go-libp2p v0.33.1-0.20240511223728-e0b67c111765 // aperture

replace github.com/ipfs/go-log/v2 => github.com/paralin/ipfs-go-logrus v0.0.0-20240410105224-e24cb05f9e98 // master

require github.com/aperturerobotics/protobuf-go-lite v0.7.0 // latest

require github.com/aperturerobotics/json-iterator-lite v1.0.0 // indirect
