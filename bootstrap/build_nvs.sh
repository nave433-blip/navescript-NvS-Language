#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
export GOTOOLCHAIN="${GOTOOLCHAIN:-local}"
export GOPROXY="${GOPROXY:-off}"
export GOSUMDB="${GOSUMDB:-off}"
mkdir -p bin
go build -mod=mod -o bin/nvs ./cmd/nvs/
./bin/nvs version
./bin/nvs run examples/selfhost.ns
./bin/nvs run examples/baseline_all.ns
echo "BOOTSTRAP OK"
