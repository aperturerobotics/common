//go:build deps_only
// +build deps_only

package hack

import (
	_ "github.com/aperturerobotics/common"

	// _ imports protowrap
	_ "github.com/aperturerobotics/goprotowrap/cmd/protowrap"
	// _ imports protoc-gen-go-lite
	_ "github.com/aperturerobotics/protobuf-go-lite/cmd/protoc-gen-go-lite"
	// _ imports protoc-gen-starpc
	_ "github.com/aperturerobotics/starpc/cmd/protoc-gen-go-starpc"

	// _ imports golangci-lint
	_ "github.com/golangci/golangci-lint/pkg/golinters"
	// _ imports golangci-lint commands
	_ "github.com/golangci/golangci-lint/pkg/commands"

	// _ imports go-mod-outdated
	_ "github.com/psampaz/go-mod-outdated"
	// _ imports goimports
	_ "golang.org/x/tools/cmd/goimports"
	// _ imports gofumpt
	_ "mvdan.cc/gofumpt"

	// _ imports goreleaser
	_ "github.com/goreleaser/goreleaser"

	// _ imports wasmbrowsertest
	_ "github.com/agnivade/wasmbrowsertest"
)
