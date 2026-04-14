# Memory Engine — Claude Guide V2

## What this repository is

A reusable memory engine for AI development systems.

It supports:

- episodic memory
- semantic memory
- heuristics
- decision ledger
- project DNA
- hybrid retrieval
- temporal validity
- graph relations
- context building
- background maintenance
- security purge
- MCP-ready read adapters

## What it is not

It is not:

- the governance core
- the system router
- the workflow engine
- the approval system
- the runtime adapter layer
- the coding agent runtime

## Required development mindset

Think like a production architect and backend engineer.
Be explicit.
Do not invent new scope.
Respect domain boundaries.
Preserve auditability.
Design for multiple projects, not one project only.

## Must-read files before coding

1. `docs/rules.md`
2. `docs/domain-invariants.md`
3. `AGENTS.md`

## Core design principles

1. Memory is not a note dump.
2. Memory is typed, scoped, temporal, relational and explainable.
3. Retrieval must be compact, relevant and bounded.
4. Historical knowledge remains queryable unless hard-purged.
5. MCP is adapter-level, not domain-level.
6. Background workers maintain quality but do not own truth.

## Before coding

Always state:

1. task understanding
2. affected modules
3. affected invariants
4. persistence impact
5. retrieval impact
6. test impact

## Technical stack

- Go
- PostgreSQL in production
- SQLite optional for local/dev
- clean / hexagonal architecture
- HTTP + SDK first
- MCP read adapter optional but planned
- background workers supported

## Output style

When implementing:

1. describe understanding
2. identify skills to use
3. show minimal implementation plan
4. implement
5. list tests
6. list assumptions or risks

## Never do this

- do not collapse all memory into one table/blob without meaning
- do not bypass temporal metadata
- do not expose raw giant memory dumps to agents
- do not make domain depend on adapters
- do not implement hard purge casually
