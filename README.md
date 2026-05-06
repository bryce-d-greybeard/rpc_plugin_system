# rpc-plugin-system

This repository uses `src/` as the source and workflow root.

- Source/workflow manifest: [`src/workflow.toml`](src/workflow.toml)
- Source/workflow feature folders: `src/<root-feature>/<feature>/`
- Per-feature workflow records live beside the code in `evidence/`, `review/`, and `postmortems/`.

Run source commands from `src/`, or use the root Makefile wrapper:

```bash
make build
make test
```
