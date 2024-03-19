module github.com/aperturerobotics/ts-common

go 1.21

replace (
	github.com/sirupsen/logrus => github.com/aperturerobotics/logrus v1.9.4-0.20240119050608-13332fb58195 // aperture
	google.golang.org/protobuf => github.com/aperturerobotics/protobuf-go v1.32.1-0.20240118233629-6aeee82c476f // aperture
)

require google.golang.org/protobuf v1.33.0
