## Common

This contains common build tools and utilities for Aperture Robotics Go projects.

See [template] for a project template that uses this package.

[template]: https://github.com/aperturerobotics/template

See [protobuf-project] for a more extensive example.

[protobuf-project]: https://github.com/aperturerobotics/protobuf-project

## Installation

The `aptre` CLI tool replaces Make for building Go projects with protobuf support.

```bash
# Run directly
go run github.com/aperturerobotics/common/cmd/aptre@latest <command>

# Or install globally
go install github.com/aperturerobotics/common/cmd/aptre@latest
```

## Usage

Start by downloading the dependencies:

```bash
bun i
```

Protobuf imports use Go paths and package names:

```protobuf
syntax = "proto3";
package example;

// Import .proto files using Go-style import paths.
import "github.com/aperturerobotics/controllerbus/controller/controller.proto";

// GetBusInfoResponse is the response type for GetBusInfo.
message GetBusInfoResponse {
  // RunningControllers is the list of running controllers.
  repeated controller.Info running_controllers = 1;
}
```

To generate the protobuf files:

```bash
git add -A
go run ./cmd/aptre generate
# or with byarn
bun run gen
```

## Commands

The `aptre` CLI provides the following commands:

| Command          | Description                                  |
| ---------------- | -------------------------------------------- |
| `generate`       | Generate protobuf code (Go, TypeScript, C++) |
| `clean`          | Remove generated files and cache             |
| `deps`           | Ensure all dependencies are installed        |
| `lint`           | Run golangci-lint                            |
| `fix`            | Run golangci-lint with --fix                 |
| `test`           | Run go test                                  |
| `test --browser` | Run tests in browser with WebAssembly        |
| `format`         | Format Go code with gofumpt                  |
| `goimports`      | Run goimports                                |
| `outdated`       | Show outdated dependencies                   |
| `release run`    | Create a release using goreleaser            |
| `release bundle` | Create a bundled snapshot release            |
| `release build`  | Build a snapshot release                     |
| `release check`  | Run goreleaser checks                        |

### Examples

```bash
# Generate protobuf files
go run ./cmd/aptre generate

# Force regeneration (ignore cache)
go run ./cmd/aptre generate --force

# Run tests
go run ./cmd/aptre test

# Run browser/WASM tests
go run ./cmd/aptre test --browser

# Lint code
go run ./cmd/aptre lint

# Format code
go run ./cmd/aptre format

# Check for outdated dependencies
go run ./cmd/aptre outdated
```

## C++ Support

C++ protobuf files (`.pb.cc` and `.pb.h`) are generated alongside the `.pb.go`
files. Add `vendor/` to your include path and create a symlink for your project:

```cmake
# CMakeLists.txt
set(VENDOR_LINK_DIR "${CMAKE_CURRENT_SOURCE_DIR}/vendor/github.com/yourorg")
if(NOT EXISTS "${VENDOR_LINK_DIR}/yourproject")
    file(CREATE_LINK "${CMAKE_CURRENT_SOURCE_DIR}" "${VENDOR_LINK_DIR}/yourproject" SYMBOLIC)
endif()

include_directories(${PROJECT_SOURCE_DIR}/vendor)
```

```cpp
#include "github.com/yourorg/yourproject/example/example.pb.h"
```

## Embedded Protoc

The `aptre` tool uses an embedded WebAssembly version of protoc via [go-protoc-wasi].
This means you don't need to install protoc separately - it works on any platform
that supports Go.

[go-protoc-wasi]: https://github.com/aperturerobotics/go-protoc-wasi

## Support

Please open a [GitHub issue] with any questions / issues.

[GitHub issue]: https://github.com/aperturerobotics/common/issues/new

... or feel free to reach out on [Matrix Chat] or [Discord].

[Discord]: https://discord.gg/KJutMESRsT
[Matrix Chat]: https://matrix.to/#/#aperturerobotics:matrix.org

## License

MIT
