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
cd /tank/development/rpc_plugin_system
make build
```

That produces:
- `.tmp-bin/rpcplugind`
- `.tmp-bin/rpcpluginctl`
- `.tmp-bin/rpcplugin-echo`
- `.tmp-bin/rpcplugin-failure`

You can also build everything with plain Go:

```bash
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

Host-managed plugin state lives under per-plugin runtime subdirectories.

## Single-plugin launch

```bash
cd /tank/development/rpc_plugin_system
.tmp-bin/rpcplugind \
  -runtime-dir /tmp/rpc_plugin_system-demo \
  -plugin ./.tmp-bin/rpcplugin-echo \
  -plugin-id echo
```

Control from another shell:

```bash
cd /tank/development/rpc_plugin_system
.tmp-bin/rpcpluginctl -runtime-dir /tmp/rpc_plugin_system-demo status
```

## Multi-plugin launch

```bash
cd /tank/development/rpc_plugin_system
.tmp-bin/rpcplugind \
  -runtime-dir /tmp/rpc_plugin_system-demo \
  -plugins echo=./.tmp-bin/rpcplugin-echo,failure=./.tmp-bin/rpcplugin-failure
```

Inspect routes and plugin state:

```bash
cd /tank/development/rpc_plugin_system
.tmp-bin/rpcpluginctl -runtime-dir /tmp/rpc_plugin_system-demo plugins
.tmp-bin/rpcpluginctl -runtime-dir /tmp/rpc_plugin_system-demo routes
.tmp-bin/rpcpluginctl -runtime-dir /tmp/rpc_plugin_system-demo plugin -plugin-id echo
```

## Packaging stance for v1

For the frozen v1 release stance, see:
- `docs/v1-freeze.md`
- `docs/release-promotion-checklist.md`

Packaging discipline for `v1.0.0` means:
- the repo builds cleanly from source
- the source release format is stable and documented
- the build outputs are named clearly
- install/run instructions are explicit
- branch promotion and release checks are explicit
- public docs match the shipped source tree and behavior
