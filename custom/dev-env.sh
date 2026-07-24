#!/usr/bin/env bash

set -euo pipefail

toolchain_root="${XDG_DATA_HOME:-${HOME}/.local/share}/sub2api-toolchain"

export PATH="${toolchain_root}/go-current/bin:${toolchain_root}/node-current/bin:${PATH}"
export GOTOOLCHAIN=local
export COREPACK_HOME="${toolchain_root}/corepack"

echo "Go:   $(go version)"
echo "Node: $(node --version)"
echo "pnpm: $(pnpm --version)"
