module github.com/aperturerobotics/ts-common

go 1.21

replace (
	github.com/sirupsen/logrus => github.com/aperturerobotics/logrus v1.9.1-0.20221224130652-ff61cbb763af // aperture
	google.golang.org/protobuf => github.com/aperturerobotics/protobuf-go v1.32.1-0.20231231025138-7d69d9b7299c // aperture
)

require google.golang.org/protobuf v1.32.0
