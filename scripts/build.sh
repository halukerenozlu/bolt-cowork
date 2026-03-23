#!/usr/bin/env bash
set -euo pipefail

echo "Building bolt-cowork..."
go build -o bolt-cowork ./cmd/bolt-cowork
echo "Build complete."
