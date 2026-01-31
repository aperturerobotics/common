# Migration to Pure Go Build System

This guide explains how to migrate projects from the old Makefile-based build
system to the new pure-Go `aptre` CLI.

## Overview

The new `aptre` CLI replaces Make with a cross-platform Go-based build tool.
Key benefits:

- **Cross-platform**: Works on any OS without requiring GNU Make, bash, or coreutils
- **Embedded protoc**: Uses WebAssembly-based protoc via [go-protoc-wasi], eliminating the need to install protobuf
- **C++ support**: Automatically generates `.pb.cc` and `.pb.h` files alongside Go and TypeScript
- **Faster caching**: Intelligent cache that tracks file hashes and tool versions
- **No MacOS setup**: No need to install GNU coreutils, gnu-sed, findutils via homebrew

[go-protoc-wasi]: https://github.com/aperturerobotics/go-protoc-wasi

## Migration Steps

### 1. Update `go.mod`

Update the `github.com/aperturerobotics/common` dependency to the `cpp` version
or later:

```bash
# Update to latest common with aptre support
go get github.com/aperturerobotics/common@latest

# Then run go mod tidy
go mod tidy
```

Add the version comment (optional, helps track versions). In go.mod, change:

```
github.com/aperturerobotics/common v0.24.1
```

To:

```
github.com/aperturerobotics/common v0.24.1 // cpp
```

### 2. Update `deps.go`

Create or update a `deps.go` file in the root of your project to ensure the
aptre CLI is available as a dependency. This file uses a build tag so it's
never actually compiled:

```go
//go:build deps_only

package yourpackage

import (
	// _ imports common with the Makefile and tools
	_ "github.com/aperturerobotics/common"
	// _ imports common aptre cli
	_ "github.com/aperturerobotics/common/cmd/aptre"
)
```

Replace `yourpackage` with your module's package name.

The `deps_only` build tag ensures this file is never compiled but the imports
are tracked by `go mod tidy`, making the aptre CLI available via `go run`.

### 3. Delete the Makefile

The Makefile is no longer needed:

```bash
rm Makefile
```

### 4. Update `package.json` Scripts

Replace Make-based scripts with `aptre` commands. Define a single `go:aptre`
script and call it from other scripts using `npm run go:aptre -- <args>`.

**Important:** If your project uses `go mod vendor`, you must add `-mod=mod` to
the `go run` command so Go can find the aptre command outside the vendor directory.

**Before:**

```json
{
  "scripts": {
    "gen": "make genproto",
    "format:go": "make format",
    "lint:go": "make lint",
    "test": "make test && npm run check"
  }
}
```

**After:**

```json
{
  "scripts": {
    "gen": "npm run go:aptre -- generate",
    "gen:force": "npm run go:aptre -- generate --force",
    "format:go": "npm run go:aptre -- format",
    "lint:go": "npm run go:aptre -- lint",
    "test": "npm run go:aptre -- test && npm run check",
    "go:aptre": "go run -mod=mod github.com/aperturerobotics/common/cmd/aptre"
  }
}
```

Full example `package.json` scripts section:

```json
{
  "scripts": {
    "build": "tsc --project tsconfig.json --noEmit false --outDir ./dist/",
    "check": "npm run typecheck",
    "typecheck": "tsc --noEmit",
    "codegen": "npm run gen",
    "ci": "npm run build && npm run lint:js && npm run lint:go",
    "format": "npm run format:go && npm run format:js && npm run format:config",
    "format:config": "prettier --write tsconfig.json package.json",
    "format:go": "npm run go:aptre -- format",
    "format:js": "npm run format:js:changed",
    "format:js:changed": "git diff --name-only --diff-filter=d HEAD | grep '\\(\\.ts\\|\\.tsx\\|\\.html\\|\\.css\\|\\.scss\\)$' | xargs -I {} prettier --write {}",
    "format:js:all": "prettier --write './!(vendor|dist)/**/(*.ts|*.tsx|*.js|*.html|*.css)'",
    "gen": "npm run go:aptre -- generate",
    "gen:force": "npm run go:aptre -- generate --force",
    "test": "npm run go:aptre -- test && npm run check && npm run test:js",
    "test:js": "vitest run",
    "lint": "npm run lint:go && npm run lint:js",
    "lint:go": "npm run go:aptre -- lint",
    "lint:js": "ESLINT_USE_FLAT_CONFIG=false eslint -c .eslintrc.cjs ./",
    "prepare": "go mod vendor && rimraf ./.tools",
    "go:aptre": "go run -mod=mod github.com/aperturerobotics/common/cmd/aptre",
    "release": "npm run release:version && npm run release:commit",
    "release:minor": "npm run release:version:minor && npm run release:commit",
    "release:version": "npm version patch -m \"release: v%s\" --no-git-tag-version",
    "release:version:minor": "npm version minor -m \"release: v%s\" --no-git-tag-version",
    "release:commit": "git reset && git add package.json && git commit -s -m \"release: v$(node -p \"require('./package.json').version\")\" && git tag v$(node -p \"require('./package.json').version\")",
    "release:publish": "git push && git push --tags",
    "precommit": "npm run format"
  }
}
```

### 5. Update GitHub Actions Workflows

Replace Make commands in CI workflows:

**Before:**

```yaml
- run: make test
- run: make lint
- run: make test-browser || true
```

**After:**

```yaml
- run: bun run go:aptre -- test
- run: bun run lint:go
- run: bun run go:aptre -- test --browser || true
```

Or use the npm scripts:

```yaml
- run: bun run test
- run: bun run lint:go
```

Also update the tools cache path:

**Before:**

```yaml
- name: Cache tools
  uses: actions/cache@v5
  with:
    path: ./tools/bin
    key: ${{ runner.os }}-tools-${{ hashFiles('tools/go.sum') }}
```

**After:**

```yaml
- name: Cache tools
  uses: actions/cache@v5
  with:
    path: ./.tools
    key: ${{ runner.os }}-aptre-tools-${{ hashFiles('**/go.sum') }}
```

### 6. Regenerate Vendor Directory

After updating dependencies:

```bash
go mod tidy
go mod vendor
```

### 7. Regenerate Proto Files

Run the generator to create the new C++ files and update existing outputs:

```bash
npm run gen:force
# or
bun run gen:force
# or directly
go run -mod=mod github.com/aperturerobotics/common/cmd/aptre generate --force
```

This will generate:

- `.pb.go` - Go protobuf code
- `.pb.ts` - TypeScript protobuf code (if package.json exists)
- `.pb.cc` / `.pb.h` - C++ protobuf code

### 8. Update .gitignore (Optional)

Add the cache file if not already present:

```gitignore
.protoc-manifest.json
```

### 9. Commit New Generated Files

The migration will create new C++ files that should be committed:

```bash
git add -A
git status  # Review new .pb.cc and .pb.h files
git commit -m "Migrate to aptre build system"
```

## Command Reference

| Old (Make)                     | New (aptre)              | npm script                           |
| ------------------------------ | ------------------------ | ------------------------------------ |
| `make genproto`                | `aptre generate`         | `npm run gen`                        |
| `make genproto-force`          | `aptre generate --force` | `npm run gen:force`                  |
| `make test`                    | `aptre test`             | `npm run go:aptre -- test`           |
| `make test-browser`            | `aptre test --browser`   | `npm run go:aptre -- test --browser` |
| `make lint`                    | `aptre lint`             | `npm run lint:go`                    |
| `make fix`                     | `aptre fix`              | `npm run go:aptre -- fix`            |
| `make format`                  | `aptre format`           | `npm run format:go`                  |
| `make gofumpt`                 | `aptre format`           | `npm run format:go`                  |
| `make goimports`               | `aptre goimports`        | `npm run go:aptre -- goimports`      |
| `make outdated`                | `aptre outdated`         | `npm run go:aptre -- outdated`       |
| `make release`                 | n/a                      | `npm run release`                    |
| `make release` (minor)         | n/a                      | `npm run release:minor`              |
| `make release-bundle`          | `aptre release bundle`   | `npm run go:aptre -- release bundle` |
| `make release-build`           | `aptre release build`    | `npm run go:aptre -- release build`  |
| `make release-check`           | `aptre release check`    | `npm run go:aptre -- release check`  |
| `make deps` / `make protodeps` | `aptre deps`             | `npm run go:aptre -- deps`           |
| `make clean-proto-cache`       | `aptre clean`            | `npm run go:aptre -- clean`          |

### Release Scripts

The release scripts use npm version to bump the version in package.json and create
a signed git commit with a tag:

```json
{
  "scripts": {
    "release": "npm run release:version && npm run release:commit",
    "release:minor": "npm run release:version:minor && npm run release:commit",
    "release:version": "npm version patch -m \"release: v%s\" --no-git-tag-version",
    "release:version:minor": "npm version minor -m \"release: v%s\" --no-git-tag-version",
    "release:commit": "git reset && git add package.json && git commit -s -m \"release: v$(node -p \"require('./package.json').version\")\" && git tag v$(node -p \"require('./package.json').version\")",
    "release:publish": "git push && git push --tags"
  }
}
```

Usage:

- `npm run release` - Create a patch release (e.g., 1.0.0 -> 1.0.1)
- `npm run release:minor` - Create a minor release (e.g., 1.0.0 -> 1.1.0)
- `npm run release:publish` - Push the release commit and tag to remote

## Using aptre Directly

You can run aptre directly without yarn/npm:

```bash
# Run from any project (requires -mod=mod if vendor directory exists)
go run -mod=mod github.com/aperturerobotics/common/cmd/aptre <command>

# Or install globally
go install github.com/aperturerobotics/common/cmd/aptre@latest
aptre <command>
```

## Troubleshooting

### "import lookup disabled by -mod=vendor"

If you see this error:

```
cannot find module providing package github.com/aperturerobotics/common/cmd/aptre: import lookup disabled by -mod=vendor
```

Add `-mod=mod` to the go run command:

```bash
go run -mod=mod github.com/aperturerobotics/common/cmd/aptre <command>
```

This is already handled if you use the `go:aptre` npm script pattern shown above.

### Tools not building

If tools fail to build, ensure the `.tools` directory has the correct go.mod:

```bash
rm -rf .tools
go run -mod=mod github.com/aperturerobotics/common .tools
```

### Cache issues

Force regeneration to rebuild the cache:

```bash
npm run gen:force
# or
npm run go:aptre -- clean && npm run gen
```

### Missing protoc plugins

The `aptre deps` command ensures all required tools are built:

```bash
npm run go:aptre -- deps --verbose
```

### Proto files not found

Ensure you run `go mod vendor` after updating dependencies so the vendor
directory contains the latest proto files from dependencies.
