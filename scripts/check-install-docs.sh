#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
runtime_dir="${TMPDIR:-/tmp}/rpc_plugin_system-install-docs-smoke-$$"
daemon_pid=""

cleanup() {
  if [[ -n "${daemon_pid}" ]] && kill -0 "${daemon_pid}" 2>/dev/null; then
    kill "${daemon_pid}" 2>/dev/null || true
    wait "${daemon_pid}" 2>/dev/null || true
  fi
  rm -rf "${runtime_dir}"
}
trap cleanup EXIT

cd "${repo_root}"
make build

for bin in rpcplugind rpcpluginctl rpcplugin-echo rpcplugin-failure; do
  test -x "src/.tmp-bin/${bin}"
done
if find src/.tmp-bin -maxdepth 1 -type f -perm /022 | grep -q .; then
  echo "build output must not be group/world writable" >&2
  exit 1
fi

(
  cd "${repo_root}/src"
  go build ./...
)

# The runtime rejects relative plugin executable paths. Keep this check aligned
# with the install guide so stale docs fail fast instead of launching a daemon
# with a shape users cannot actually run.
if "${repo_root}/src/.tmp-bin/rpcplugind" \
  -runtime-dir "${runtime_dir}-relative" \
  -plugin ./src/.tmp-bin/rpcplugin-echo \
  -plugin-id echo 2>"${runtime_dir}-relative.err"; then
  echo "expected relative plugin path rejection" >&2
  exit 1
fi
grep -q "plugin executable path must be absolute" "${runtime_dir}-relative.err"
rm -f "${runtime_dir}-relative.err"

"${repo_root}/src/.tmp-bin/rpcplugind" \
  -runtime-dir "${runtime_dir}" \
  -plugin "${repo_root}/src/.tmp-bin/rpcplugin-echo" \
  -plugin-id echo &
daemon_pid=$!

for _ in $(seq 1 50); do
  if "${repo_root}/src/.tmp-bin/rpcpluginctl" -runtime-dir "${runtime_dir}" status >/dev/null 2>&1; then
    exit 0
  fi
  sleep 0.1
done

echo "timed out waiting for documented daemon/ctl smoke status" >&2
exit 1
