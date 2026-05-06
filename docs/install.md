# Install and packaging guide

This project releases source only.

That means:
- build from source locally
- run the binaries directly
- release only in a stable format
- treat packaging as a disciplined source-layout and release story, not as distro packaging

## Requirements

Current baseline:
- Go `1.26.2`
- Linux is the first-class local runtime target for v1
- Unix domain sockets are required

Current notes:
- Linux peer credential verification is supported in v1 through `SO_PEERCRED`
- non-Linux builds may compile, but Linux is the first supported hardening target
- post-v1 may add BSD and other Unix peer credential backends through the same adapter pattern

## Build from source

From the repo root:

```bash
REPO=/path/to/rpc-plugin-system
cd "$REPO"
make build
```

The root `Makefile` delegates to the Go source root under `src/`. That produces private, non-group/world-writable executables:
- `src/.tmp-bin/rpcplugind`
- `src/.tmp-bin/rpcpluginctl`
- `src/.tmp-bin/rpcplugin-echo`
- `src/.tmp-bin/rpcplugin-failure`

You can also build everything with plain Go from the source root:

```bash
cd "$REPO/src"
go build ./...
```

## Minimal runtime layout

The system expects:
- one runtime directory writable by the operator process
- executable plugin paths
- Unix socket support

Example runtime layout during execution:
- `/tmp/rpc_plugin_system-demo/admin.sock`
- `/tmp/rpc_plugin_system-demo/echo/echo.sock`
- `/tmp/rpc_plugin_system-demo/echo/events.jsonl`
- `/tmp/rpc_plugin_system-demo/echo/plugin-events.jsonl`

Host-managed plugin state lives under per-plugin runtime subdirectories.

Plugin id notes:
- plugin ids become runtime subdirectory names and socket/auth file prefixes
- for that reason, plugin ids are intentionally restricted to path-safe names
- allowed characters are letters, digits, dot (`.`), underscore (`_`), and dash (`-`)
- plugin ids must begin with a letter or digit

## Single-plugin launch

```bash
cd "$REPO"
src/.tmp-bin/rpcplugind \
  -runtime-dir /tmp/rpc_plugin_system-demo \
  -plugin "$(pwd)/src/.tmp-bin/rpcplugin-echo" \
  -plugin-id echo
```

Control from another shell:

```bash
cd "$REPO"
src/.tmp-bin/rpcpluginctl -runtime-dir /tmp/rpc_plugin_system-demo status
```

## Multi-plugin launch

```bash
cd "$REPO"
src/.tmp-bin/rpcplugind \
  -runtime-dir /tmp/rpc_plugin_system-demo \
  -plugins "echo=$(pwd)/src/.tmp-bin/rpcplugin-echo,failure=$(pwd)/src/.tmp-bin/rpcplugin-failure"
```

Inspect routes and plugin state:

```bash
cd "$REPO"
src/.tmp-bin/rpcpluginctl -runtime-dir /tmp/rpc_plugin_system-demo plugins
src/.tmp-bin/rpcpluginctl -runtime-dir /tmp/rpc_plugin_system-demo routes
src/.tmp-bin/rpcpluginctl -runtime-dir /tmp/rpc_plugin_system-demo plugin -plugin-id echo
```

Log inspection note:
- in single-plugin mode, `rpcpluginctl logs` can fall back to the sole kernel plugin log automatically
- in multi-plugin mode, pass `-plugin-id` explicitly when reading kernel logs
- plugin-side SDK logs live in sibling `plugin-events.jsonl` files and remain an intentional shell-first inspection path for v1

## Install docs smoke check

A lightweight check for the documented build/run command shape is available from the repo root:

```bash
scripts/check-install-docs.sh
```

The check builds through the root `Makefile`, verifies the `src/.tmp-bin` outputs, verifies plain `go build ./...` from `src/`, confirms relative plugin executable paths are rejected, then starts the daemon with an absolute plugin path only long enough for `rpcpluginctl status` to respond.

## Packaging stance for v1

For the frozen v1 release stance, see:
- `docs/v1-freeze.md`
- `docs/release-promotion-checklist.md`

Packaging discipline for `v1.0.0` means:
- no late v1 feature-scope expansion
- the repo builds cleanly from source
- the source release format is stable and documented
- the build outputs are named clearly
- install/run instructions are explicit
- branch promotion and release checks are explicit
- public docs match the shipped source tree and behavior


## Plugin path requirements

Configured plugin executables must use absolute paths. Relative paths, bare command names, symlinks, directories, non-executable files, and group/world-writable executables are rejected.
