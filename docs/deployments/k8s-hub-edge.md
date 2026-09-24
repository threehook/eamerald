# Kubernetes hub + edge deployment

This document describes the current Eamerald Helm charts and the two edge packaging options we support.

## Charts (`k8s/eamerald-hub`, `k8s/eamerald-edge`, `k8s/eamerald-standalone`)

Three Helm charts, sharing common templates via the `k8s/eamerald-common` library chart:

| Chart | Runs | Store | Replicas |
|------|------|--------|----------|
| **eamerald-hub** | Directory only (source of truth) | Bolt (default) or Postgres | `replicaCount > 1` only with Postgres |
| **eamerald-edge** | Authorizer + local directory cache | Per-pod disposable store | Many OK; sync from hub |
| **eamerald-standalone** | Directory + authorizer + console, all in one | Bolt only | 1 (no HA story) |

**eamerald-hub + eamerald-edge is the default local topology** (`make k8s-deploy`). **eamerald-standalone** is a legacy, last-resort option for a quick single-instance check that doesn't need the hub/edge split (`make k8s-deploy-standalone`) - it has no HA story and can't scale the authorizer independently of the directory.

**Hub** owns the directory data (Bolt or Postgres) and exports it on directory gRPC (port `9292`). Edge installs set `edge.hub.address` to that Service.

**Edge** runs an **authorizer** with a local directory cache. It does not seed a manifest and does not keep a durable DB PVC for that cache. The `aserto_edge` plugin pulls from the hub on an interval (`edge.syncIntervalMinutes`, default 1) - its readinessProbe is pinned to that sync, so it's excluded from its Service until the first sync lands. Rolling updates are always safe (no shared Bolt file lock).

**Also worth knowing:** an init container seeds the manifest for hub/standalone; PVCs apply only to Bolt-backed hub/standalone. None of the three charts ever use `strategy.type: Recreate` - a Bolt-backed hub or standalone instead uses `RollingUpdate` with `maxSurge: 0, maxUnavailable: 1`, which forces the old Pod to fully terminate before the new one starts (avoiding two writers on the same Bolt file) without ever changing `strategy.type` between releases.

Sidecar packaging for edge lives under [`sidecar-deployment/`](sidecar-deployment/) (copy the edge container into an app Deployment).

## Source of truth and sync

- **Source of truth:** the hub directory (and its store — Bolt or Postgres).
- **Who exports:** the hub (directory Export API).
- **Who pulls:** each edge authorizer (`aserto_edge`), into its **own** local store.
- **Apps** call an authorizer (shared edge Service or localhost sidecar). They do **not** talk to the hub database.

## Architecture A — Shared edge Deployment (ClusterIP)

Apps call a shared edge Service of **authorizers**. Those authorizers sync from the hub (Postgres-backed directory).

**Draw.io:** [edge-shared-deployment.drawio](diagrams/edge-shared-deployment.drawio)

```mermaid
flowchart LR
  subgraph Hub
    H[Hub directory]
    PG[(Postgres)]
    H --- PG
  end

  subgraph EdgeDeployment["Edge Deployment"]
    E1[Edge authorizer<br/>(eameraldd)]
    E2[Edge authorizer<br/>(eameraldd)]
  end

  A1[App A] --> E1
  A2[App B] --> E1
  A3[App C] --> E2

  E1 -. sync .-> H
  E2 -. sync .-> H
```

**Chart usage:** install `eamerald-hub`, then one or more `eamerald-edge` releases pointing at the hub address (`make k8s-deploy` does exactly this for local dev).

## Architecture B — Sidecar edge (in each app pod)

Each app pod embeds an **authorizer** sidecar; the app talks to localhost; each authorizer syncs from the hub (Postgres-backed directory).

**Draw.io:** [edge-sidecar-deployment.drawio](diagrams/edge-sidecar-deployment.drawio)

```mermaid
flowchart LR
  subgraph Hub
    H[Hub directory]
    PG[(Postgres)]
    H --- PG
  end

  subgraph Pod1["App pod 1"]
    A1[App]
    S1[Edge authorizer<br/>(eameraldd)]
    A1 --> S1
  end

  subgraph Pod2["App pod 2"]
    A2[App]
    S2[Edge authorizer<br/>(eameraldd)]
    A2 --> S2
  end

  S1 -. sync .-> H
  S2 -. sync .-> H
```

**Packaging:** use the example under [`sidecar-deployment/eamerald-edge-sidecar.yaml`](sidecar-deployment/eamerald-edge-sidecar.yaml).

## Both are supported

| | Shared edge Deployment | Sidecar |
|--|------------------------|---------|
| **Check() path** | Cluster network to edge Service | Localhost in the app pod |
| **Ops** | Few edge replicas to run | One edge per app pod |
| **Best for** | Default platform / most apps | Hot paths that need in-process locality |

Default recommendation for most customers: **shared edge Deployment**. Offer **sidecar** when an app needs localhost checks.
