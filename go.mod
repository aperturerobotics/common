module github.com/aperturerobotics/common

go 1.22

// This fork uses protobuf-go-lite. This replace can be safely removed but optimizes binary size.
replace github.com/libp2p/go-libp2p => github.com/aperturerobotics/go-libp2p v0.33.1-0.20240504075939-591fc65373be // aperture

require github.com/aperturerobotics/protobuf-go-lite v0.6.1 // latest

require github.com/aperturerobotics/json-iterator-lite v1.0.0 // indirect
