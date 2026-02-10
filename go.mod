module github.com/aperturerobotics/common

go 1.25

// This fork uses protobuf-go-lite. This replace can be safely removed but optimizes binary size.
replace github.com/libp2p/go-libp2p => github.com/aperturerobotics/go-libp2p v0.37.1-0.20241111002741-5cfbb50b74e0 // aperture

replace github.com/ipfs/go-log/v2 => github.com/paralin/ipfs-go-logrus v0.0.0-20240410105224-e24cb05f9e98 // master

require (
	github.com/aperturerobotics/abseil-cpp v0.0.0-20260131110040-4bb56e2f9017 // aperture-2
	github.com/aperturerobotics/cli v1.1.0
	github.com/aperturerobotics/go-protoc-gen-prost v0.0.0-20260204215916-dc1f0fed8cfc // master
	github.com/aperturerobotics/go-protoc-wasi v0.0.0-20260131050911-b5f94b044584 // master
	github.com/aperturerobotics/json-iterator-lite v1.0.1-0.20251104042408-0c9eb8a3f726 // indirect
	github.com/aperturerobotics/protobuf v0.0.0-20260203024654-8201686529c4 // wasi
	github.com/aperturerobotics/protobuf-go-lite v0.12.1 // latest
	github.com/aperturerobotics/starpc v0.46.2 // master
	github.com/aperturerobotics/util v1.32.3 // indirect
)

require (
	github.com/coder/websocket v1.8.14 // indirect
	github.com/ipfs/go-cid v0.4.1 // indirect
	github.com/klauspost/cpuid/v2 v2.2.8 // indirect
	github.com/libp2p/go-buffer-pool v0.1.0 // indirect
	github.com/libp2p/go-libp2p v0.47.0 // indirect
	github.com/libp2p/go-yamux/v4 v4.0.2 // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/mr-tron/base58 v1.2.0 // indirect
	github.com/multiformats/go-base32 v0.1.0 // indirect
	github.com/multiformats/go-base36 v0.2.0 // indirect
	github.com/multiformats/go-multiaddr v0.13.0 // indirect
	github.com/multiformats/go-multibase v0.2.0 // indirect
	github.com/multiformats/go-multihash v0.2.3 // indirect
	github.com/multiformats/go-multistream v0.5.0 // indirect
	github.com/multiformats/go-varint v0.0.7 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/sirupsen/logrus v1.9.4 // indirect
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/tetratelabs/wazero v1.11.0
	github.com/xrash/smetrics v0.0.0-20250705151800-55b8f293f342 // indirect
	golang.org/x/crypto v0.45.0 // indirect
	golang.org/x/exp v0.0.0-20250305212735-054e65f0b394 // indirect
	golang.org/x/mod v0.33.0
	golang.org/x/sys v0.40.0 // indirect
	lukechampine.com/blake3 v1.3.0 // indirect
)
