# Architecture Overview

## Layers

### Domain

Contains:

- entities
- value objects
- domain services
- invariants

Must not depend on:

- HTTP
- DB drivers
- MCP
- frameworks

### Application

Contains:

- use cases
- orchestration of domain operations
- transaction boundaries
- context building
- worker jobs

### Ports

Contains:

- repository interfaces
- search provider interfaces
- graph provider interfaces
- embedding provider interfaces
- event publisher interfaces

### Adapters

Contains:

- inbound: HTTP, SDK, MCP
- outbound: persistence, search, vector, graph, events

### Infrastructure

Contains:

- config
- logger
- DB wiring
- migrations
- scheduler
- queue
- observability

## High-level flow

governance-core
→ memory-engine inbound API/SDK
→ application use case
→ domain validation
→ outbound ports
→ persistence / search / graph / vector
→ compact result back to caller
