#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"

required_docs=(docs/compatibility.md docs/plugin-standard-v0.md)
for doc in "${required_docs[@]}"; do
  test -f "${doc}"
done

grep -q "v1 does not define a wire protocol version field" docs/compatibility.md
grep -q "v1 does not define a wire protocol version field" docs/plugin-standard-v0.md

forbidden_patterns=(
  "protocol version where applicable later"
  "^[[:space:]]*-[[:space:]]*protocol version$"
  "Protocol version must be compatible"
  "future protocol/version variables"
  "protocol version incompatibility"
  "plugin standard / protocol version"
  "compare protocol version compatibility"
)

for pattern in "${forbidden_patterns[@]}"; do
  if grep -RInE "${pattern}" docs; then
    echo "docs promise protocol-version wire fields/checks that v1 does not implement" >&2
    exit 1
  fi
done
