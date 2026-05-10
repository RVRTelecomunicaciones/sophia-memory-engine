# syntax=docker/dockerfile:1.7

# ---- build stage (shared) -----------------------------------------------
# One build stage compiles all binaries; per-binary runtime stages select
# the artifact via --target. Mirrors the pattern used by sophia-orchestator
# and sophia-runtime-adapters in the Sophia ecosystem.
FROM golang:1.26.2-alpine AS build

# Build tools needed only inside the builder.
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /src

# Cache module downloads separately from source compilation.
COPY go.mod go.sum ./
RUN go mod download

# Source.
COPY cmd/      cmd/
COPY internal/ internal/
COPY migrations/ migrations/

# CGO_ENABLED=0 + trimpath + stripped symbols → static binaries fit for
# distroless/static. `-s -w` strips symbol + DWARF info; `-trimpath`
# removes absolute paths from the binary so reproducibility improves.
ARG VERSION=dev
ARG COMMIT=unknown
ENV CGO_ENABLED=0 GOOS=linux

RUN go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /out/memory-engine \
    ./cmd/memory-engine

RUN go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /out/workers \
    ./cmd/workers

# ---- memory-engine runtime stage ----------------------------------------
# Distroless/static: ~2 MiB base, no shell, no package manager, nonroot user.
# Migrations are bundled so the container can self-migrate when paired with
# the cmd/migrate binary in a sidecar / init container (see compose stack).
# CA certs and zoneinfo are copied because pgx may perform datetime
# conversions that depend on /usr/share/zoneinfo and outbound TLS in some
# deployments uses the system trust store.
FROM gcr.io/distroless/static-debian12:nonroot AS memory-engine

COPY --from=build /out/memory-engine                   /usr/local/bin/memory-engine
COPY --from=build /src/migrations                      /var/memory/migrations
COPY --from=build /etc/ssl/certs/ca-certificates.crt   /etc/ssl/certs/
COPY --from=build /usr/share/zoneinfo                  /usr/share/zoneinfo

ENV MEMORY_ENGINE_MIGRATIONS_PATH=/var/memory/migrations/postgres \
    TZ=UTC

USER nonroot:nonroot

EXPOSE 8080

# Distroless static has no shell, so HEALTHCHECK cannot run an inline
# script. Compose / k8s should poll /health from outside the container.

ENTRYPOINT ["/usr/local/bin/memory-engine"]

# ---- workers runtime stage ----------------------------------------------
# The workers binary runs the async jobs (importance recompute, freshness,
# consolidation, project DNA, contradiction detection, purge cleanup).
# Independent lifecycle from the memory-engine HTTP service: separate
# container, separate restart, separate health.
FROM gcr.io/distroless/static-debian12:nonroot AS workers

COPY --from=build /out/workers                         /usr/local/bin/workers
COPY --from=build /etc/ssl/certs/ca-certificates.crt   /etc/ssl/certs/
COPY --from=build /usr/share/zoneinfo                  /usr/share/zoneinfo

ENV TZ=UTC

USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/workers"]
