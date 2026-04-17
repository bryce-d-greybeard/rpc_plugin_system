# Future Architecture Constraints

This document records design constraints that later versions must preserve without expanding v0.1.0 scope.

## Storage direction

- Postgres will become the canonical truth store.
- Canonical notes will be lossless TOML sticky notes.
- Exact chunks and provenance must be preserved.
- Summary-first storage is forbidden.

## Retrieval direction

- Retrieval hierarchy should evolve as groups -> entities -> chunks.
- Contextual chunks should normalize around roughly 512 tokens.
- Smaller atomic note types may remain below that size.
- Links such as support, contradiction, supersedes, same-topic, and same-source matter.

## Vector direction

- pgvector will be a locator layer only.
- Embeddings must point to canonical note IDs stored in Postgres.
- Vector search must not become the source of truth.

## Planner direction

- If MCTS or another planner is added, it is a second-stage retrieval planner.
- Planning must not replace first-pass retrieval or canonical storage.

## Compatibility rule

Future work must not casually break the v0.1.0 kernel and plugin lifecycle model:
- plugins remain separate executables
- supervision remains kernel-owned
- generation boundaries remain strict
- dead plugin identity is never healed in place
