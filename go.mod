module github.com/aperturerobotics/ts-common

go 1.21

replace (
	github.com/sirupsen/logrus => github.com/aperturerobotics/logrus v1.9.4-0.20240119050608-13332fb58195 // aperture
	google.golang.org/protobuf => github.com/aperturerobotics/protobuf-go v1.33.1-0.20240411062030-e36f75e0a3b8 // aperture
)

require (
	github.com/aperturerobotics/protobuf-go-lite v0.2.3
	google.golang.org/protobuf v1.33.0
)
