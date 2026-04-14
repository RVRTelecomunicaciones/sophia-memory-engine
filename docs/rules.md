# Project Rules V2

## 1. Repository purpose

This repository implements a reusable memory engine for AI development systems.

It is responsible for:

- episodic memory
- semantic memory
- heuristics
- decision ledger
- project DNA
- hybrid retrieval
- temporal validity
- graph relationships
- context building
- background maintenance
- security purge
- MCP-ready read layer

It is NOT responsible for:

- workflow orchestration
- global routing of the system
- policy enforcement of the whole platform
- approvals
- PR/merge/deploy
- runtime execution of coding agents

## 2. Core invariants

1. Episodic memory != semantic memory.
2. Heuristics != raw memories.
3. Decisions are formal and auditable.
4. Every record must have:
   - type
   - scope
   - provenance
   - temporal metadata
5. Retrieval must return compact and explainable context.
6. Graph traversal must be bounded.
7. Historical knowledge must remain queryable unless hard-purged.
8. Hard purge is exceptional and compliance-driven.
9. Project DNA is derived knowledge, not primary truth.
10. Background workers must not violate domain invariants.

## 3. Multi-scope rule

All relevant memory structures must support at least:

- project_id
- repo_id
- agent_id
- session_id or run_id
- environment
- optional user_id

## 4. Retrieval rule

Retrieval must support:

- keyword / FTS
- semantic search abstraction
- graph expansion
- temporal filters
- scope filters
- retrieval strategy selection

Do not build plain-text-only retrieval.

## 5. Heuristic rule

Every heuristic must contain:

- condition
- action
- rationale
- scope
- confidence
- enabled flag
- validity / expiry

## 6. Decision rule

A decision:

- may be active
- may be superseded
- may be contradicted
- may be archived
- must never disappear silently

## 7. Project DNA rule

Project DNA must be generated from:

- active decisions
- heuristics
- episodic patterns
- architecture signals
- repeated findings

It must be refreshable and versionable.

## 8. Background workers rule

Workers are allowed to:

- consolidate
- detect contradictions
- refresh freshness
- recompute importance
- prebuild project DNA

Workers are NOT allowed to:

- silently rewrite decisions
- destroy critical evidence
- auto-purge secure content without explicit policy/use case

## 9. Hard purge rule

Hard purge is only for:

- secrets leakage
- compliance-mandated deletion
- highly sensitive content removal

Hard purge must:

- delete or destroy content
- invalidate vectors/embeddings
- invalidate caches/indexes
- preserve only minimal safe audit metadata

## 10. MCP rule

MCP support is adapter-level.
The domain must not depend on MCP concepts.

The read layer may expose:

- project profile
- heuristics
- decision timeline
- context bundle
- memory search resources/prompts

## 11. Testing rule

Every non-trivial change requires:

- unit tests
- integration tests when persistence/retrieval changes
- temporal tests when validity logic changes
- graph tests when relation logic changes
- worker tests when maintenance behavior changes
- purge tests when security deletion changes

## 12. Simplicity rule

Do not:

- overengineer phase 1
- introduce distributed microservices
- force vector DB on day 1
- let MCP drive domain design
- create modules without domain pressure
