#!/bin/sh
# Proves a new gosec finding fails the linter. The fixture is outside the main packages.
set -eu
root="$(CDPATH= cd -- "$(dirname "$0")" && pwd)"
if command -v golangci-lint >/dev/null 2>&1; then
  lint=golangci-lint
else
  echo "golangci-lint is not installed; skipping local probe"
  exit 0
fi
if "$lint" run --disable-all --enable=gosec "$root" >/tmp/gosec-probe.out 2>&1; then
  echo "gosec accepted a known weak hash"
  exit 1
fi
echo "gosec rejected the fixture"
