#!/bin/sh
# Spúšťač testov Go pre prehliadač (go test -exec): Node.js s pripraveným
# SQLite vo WebAssembly a s malým prostredím (wasm_exec má limit dĺžky).
dir=$(cd "$(dirname "$0")" && pwd)
exec env -i PATH="$PATH" HOME="$HOME" TMPDIR="${TMPDIR:-/tmp}" \
  node --stack-size=8192 --import="$dir/go-wasm-sqlite.mjs" "$(go env GOROOT)/lib/wasm/wasm_exec_node.js" "$@"
