# Agent Skills Index V2

Every non-trivial task must use one or more skills.

## Available skills

- architecture-guardrails
- domain-modeling
- retrieval-engine
- retrieval-routing
- graph-modeling
- temporal-memory
- heuristics-engine
- decision-ledger
- project-dna
- consolidation-archival
- background-workers
- security-purge
- api-contracts
- mcp-read-layer
- persistence-postgres
- testing-quality

## Usage rules

1. Use architecture-guardrails whenever boundaries may change.
2. Use testing-quality for every non-trivial change.
3. Use retrieval-engine whenever search behavior changes.
4. Use retrieval-routing whenever retrieval strategy selection changes.
5. Use background-workers for async maintenance logic.
6. Use security-purge for purge workflows or secret-removal logic.

## Task mapping

### New entity / domain rule

- domain-modeling
- architecture-guardrails
- testing-quality

### Retrieval or context changes

- retrieval-engine
- retrieval-routing
- temporal-memory
- graph-modeling
- testing-quality

### Heuristic changes

- heuristics-engine
- decision-ledger
- testing-quality

### Decision ledger changes

- decision-ledger
- temporal-memory
- testing-quality

### Project DNA changes

- project-dna
- consolidation-archival
- testing-quality

### Worker changes

- background-workers
- architecture-guardrails
- testing-quality

### Purge/compliance changes

- security-purge
- persistence-postgres
- testing-quality

### MCP read exposure

- mcp-read-layer
- api-contracts
- testing-quality
