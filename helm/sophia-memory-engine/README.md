# sophia-memory-engine Helm chart

Knowledge layer for the Sophia ecosystem. Wires the HTTP `memory-engine`
server target (workers async target shipped separately).

## Quick install

```bash
helm install memory ./helm/sophia-memory-engine \
  --set secrets.dbHost=pg-memory-engine \
  --set secrets.dbUser=memory_engine \
  --set secrets.dbPassword=$(vault kv get -field=password secret/sophia/memory) \
  --set secrets.dbName=memory_engine \
  --set secrets.dbSslMode=require
```

## Production checklist

- [ ] `secrets.*` come from External Secrets Operator (Vault, AWS SM, GCP SM) — NEVER commit values
- [ ] Set `secrets.dbSslMode: require` (never `disable` outside dev)
- [ ] Adjust `resources.limits` based on observed RPS + retrieval workload
- [ ] If running migrations out-of-band, set `migrate.enabled: false`
- [ ] Add `podAnnotations` for `vault-agent-injector` if applicable
- [ ] Pin `image.tag` to a digest, not a mutable tag, for reproducibility

## Probes

- Liveness: `GET /health` — basic process aliveness
- Readiness: `GET /ready` — includes DB connectivity check

## Auth

memory-engine validates `X-API-Key` against sha256-hashed entries in
`api_keys` table. Bootstrap key seeding (via migration 003) is for dev;
production operators rotate keys out-of-band.

## See also

- `sophia-orchestator/helm/` — dispatcher service that calls memory-engine
- `sophia-memory-engine/Dockerfile` — multi-target image (memory-engine + workers)
- `docs/operations/llm-providers.md` (in orchestator) — how memory-engine fits in the SDD cycle
