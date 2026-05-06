# rpc-plugin-system

This repository uses `src/` as the source and workflow root.

- Source/workflow root: [`src/`](src/)
- Workflow manifest: [`src/workflow.toml`](src/workflow.toml)
- Workflow folders: [`src/workflow/`](src/workflow/)
- Source feature folders: `src/<root-feature>/<feature>/`

Run source commands from `src/`, or use the root Makefile wrapper:

```bash
make build
make test
```
