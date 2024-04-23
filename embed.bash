#!/bin/bash
set -eo pipefail
set -x

cp ./tools/go.mod ./go.mod.tools
cp ./tools/go.sum ./go.sum.tools
cp ./tools/deps.go ./deps.go.tools
sed -i '/github.com\/aperturerobotics\/common/d' go.mod.tools go.sum.tools deps.go.tools
