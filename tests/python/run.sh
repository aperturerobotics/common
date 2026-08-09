#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
CHECK_DIR="$ROOT/tests/python"
TMP=$(mktemp -d "${TMPDIR:-/tmp}/common-python-compatibility.XXXXXX")
trap 'rm -rf "$TMP"' EXIT
export UV_PROJECT_ENVIRONMENT="$TMP/.venv"
export MYPY_CACHE_DIR="$TMP/.mypy_cache"

cat >"$TMP/go.mod" <<EOF
module example.com/compatibility

go 1.25.0

require github.com/aperturerobotics/common v0.0.0

replace github.com/aperturerobotics/common => $ROOT
EOF
cp "$ROOT/example/compatibility.proto" "$ROOT/example/compatibility_options.proto" "$TMP/"
mkdir -p "$TMP/vendor/github.com/aperturerobotics/protobuf/src/google/protobuf"
cp "$ROOT/vendor/github.com/aperturerobotics/protobuf/src/google/protobuf/timestamp.proto" \
  "$ROOT/vendor/github.com/aperturerobotics/protobuf/src/google/protobuf/descriptor.proto" \
  "$TMP/vendor/github.com/aperturerobotics/protobuf/src/google/protobuf/"
git -C "$TMP" init -q
git -C "$TMP" add compatibility.proto compatibility_options.proto

# Dependency setup is intentionally enabled: Python-only planning must not create
# native tool or Node dependencies, while the embedded protoc reactor remains usable.
go run "$ROOT/cmd/aptre" generate --project-dir "$TMP" --language python --rpc none

python3 - "$TMP" <<'PY'
import pathlib
import sys

root = pathlib.Path(sys.argv[1])
want = {
    "compatibility_pb2.py",
    "compatibility_pb2.pyi",
    "compatibility_options_pb2.py",
    "compatibility_options_pb2.pyi",
}
got = {p.name for p in root.glob("*_pb2.py")} | {p.name for p in root.glob("*_pb2.pyi")}
if got != want:
    raise SystemExit(f"generated Python outputs = {sorted(got)!r}, want {sorted(want)!r}")
if (root / ".tools").exists() or (root / "node_modules").exists():
    raise SystemExit("Python-only generation created unrelated dependencies")
PY

PYTHONPATH="$TMP" uv run --directory "$CHECK_DIR" python "$CHECK_DIR/consumer.py" >/dev/null
MYPYPATH="$TMP" PYTHONPATH="$TMP" uv run --directory "$CHECK_DIR" mypy --strict --no-incremental "$CHECK_DIR/consumer.py"

mkdir -p "$TMP/python-output"
cp "$TMP"/*_pb2.py "$TMP"/*_pb2.pyi "$TMP/python-output/"
