# Domain Invariants

## MemoryRecord invariants

- must have id
- must have type
- must have provenance
- must have at least one scope
- must have created_at
- must have validity metadata or explicit timeless semantics
- must not mix decision data and heuristic data in the same semantic role

## DecisionRecord invariants

- must include title, decision, rationale
- must include scope
- must include confidence
- must be traceable to evidence or source
- can be active, superseded, contradicted, archived
- must never disappear silently

## HeuristicRule invariants

- must include condition
- must include action
- must include rationale
- must include confidence
- must include scope
- must be enable/disable capable
- must support expiry or reviewability

## Relation invariants

- relation type must be explicit
- source and target must exist unless hard-purged
- traversal must be bounded by depth or budget

## Purge invariants

- hard purge is irreversible
- hard purge must invalidate derived artifacts
- hard purge must leave only safe audit metadata
