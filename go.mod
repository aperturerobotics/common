module github.com/aperturerobotics/ts-common

go 1.22

toolchain go1.22.2

replace (
	github.com/sirupsen/logrus => github.com/aperturerobotics/logrus v1.9.4-0.20240119050608-13332fb58195 // aperture
	google.golang.org/protobuf => github.com/aperturerobotics/protobuf-go v1.33.1-0.20240411062030-e36f75e0a3b8 // aperture
)

require (
	github.com/aperturerobotics/protobuf-go-lite v0.3.1
	github.com/pkg/errors v0.9.1
)

require (
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/modern-go/concurrent v0.0.0-20180228061459-e0a39a4cb421 // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
)
