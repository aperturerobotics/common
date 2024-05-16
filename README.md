## Common

This contains common files like the project Makefile.

See [template] for a project template that uses this package.

[template]: https://github.com/aperturerobotics/template

See [protobuf-project] for a more extensive example.

[protobuf-project]: https://github.com/aperturerobotics/protobuf-project

## Usage

Start by downloading the dependencies:

```bash
yarn
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
$ git add -A
$ yarn gen
```

The Makefile will download the tools using Go to a bin dir.

## Makefile

The available make targets are:

 - `gen`: Generate protobuf files.
 - `test`: Run go tests. 
 - `test-browser`: Run go tests in a web browser with WebAssembly.
 - `lint`: Run golangci-lint on the project.
 - `fix`: Run golangci-lint with --fix on the project.
 - `format`: Format Go and TypeScript code.
 - `release`: Create a new release using goreleaser.
 - `release-bundle`: Create a bundled snapshot release using goreleaser. 
 - `release-build`: Build a snapshot release using goreleaser.
 - `release-check`: Run goreleaser checks.

To generate the TypeScript and Go code:

 - `yarn gen`

To format the Go and TypeScript files:

 - `yarn format`

## Eject

You can "eject" and copy all the project files directly to your repo:

```
# NOTE: not a full list of files
cp ./vendor/github.com/aperturerobotics/common/{Makefile,.eslintrc.js,.eslintignore} ./
```

While not implemented yet, "make eject" will do this for you.

## Developing on MacOS

On MacOS, some homebrew packages are required for `yarn gen`:

```
brew install bash make coreutils gnu-sed findutils protobuf
brew link --overwrite protobuf
```

Add to your .bashrc or .zshrc:

```
export PATH="/opt/homebrew/opt/coreutils/libexec/gnubin:$PATH"
export PATH="/opt/homebrew/opt/gnu-sed/libexec/gnubin:$PATH"
export PATH="/opt/homebrew/opt/findutils/libexec/gnubin:$PATH"
export PATH="/opt/homebrew/opt/make/libexec/gnubin:$PATH"
```

## Support

Please open a [GitHub issue] with any questions / issues.

[GitHub issue]: https://github.com/aperturerobotics/common/issues/new

... or feel free to reach out on [Matrix Chat] or [Discord].

[Discord]: https://discord.gg/KJutMESRsT
[Matrix Chat]: https://matrix.to/#/#aperturerobotics:matrix.org

## License

MIT
