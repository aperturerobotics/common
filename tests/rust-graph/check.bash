#!/bin/bash
set -eo pipefail

# Generates the whole-graph Rust output of this project as a consumer would,
# checks that it is current, lints it, and runs the crate tests over exactly that
# output with cargo-nextest.
#
# Usage: tests/rust-graph/check.bash
#
#   APTRE_REF  git ref of aperturerobotics/common whose aptre generates the
#              output, default master. Set it to a pushed branch or commit to
#              check a change before it lands.
#
# The project is copied into a scratch Git repository outside this tree, so
# aptre prepares its own dependencies, including the Prost plugin and the
# StarPC service plugin, from the published modules the way a downstream
# project's first run does. Nothing in the run is replaced or patched.

root=$(cd "$(dirname "$0")/../.." && pwd)
ref="${APTRE_REF:-master}"
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT

# The schemas must be tracked by Git, which is how aptre discovers them.
project="$scratch/rust-graph"
mkdir -p "$project"
(cd "$root" && git ls-files -z --cached --others --exclude-standard tests/rust-graph) |
  while IFS= read -r -d '' file; do
    mkdir -p "$project/$(dirname "${file#tests/rust-graph/}")"
    cp "$root/$file" "$project/${file#tests/rust-graph/}"
  done
git -C "$project" init --quiet
git -C "$project" add .

# Run the published aptre at the ref, resolved by the Go module proxy.
aptre="github.com/aperturerobotics/common/cmd/aptre@$ref"
go run "$aptre" generate -C "$project"
go run "$aptre" generate --check -C "$project"

# Generated code must pass the lints that consumers deny.
cargo clippy --manifest-path "$project/Cargo.toml" --all-targets -- -D warnings

cargo nextest run --manifest-path "$project/Cargo.toml" \
  --config-file "$project/.config/nextest.toml"
