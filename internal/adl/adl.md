# Authorization Decision Log

Writes one record per authorization decision, conforming to the Logius ADL
1.0 Level 1 record shape:
https://gitdocumentatie.logius.nl/publicatie/ftv/adl/1.0.0/

Every evaluated request produces exactly one record — including requests the
PDP failed to evaluate, which are recorded with `status: Error`.

Supports two outputs, independently or together (see `output` below): a JSON
line to stdout, meant to be scraped by a log-shipping agent (e.g. Grafana
Alloy) and forwarded to Loki; and/or a direct OTLP log export, which the
spec recommends as the transport between the application and the log.

## Scope

One `*adl.Logger` is built at daemon startup and shared by every service
that evaluates decisions, so a deployment running both does not open two
OTLP exporters:

| Endpoint | `event_name` |
|---|---|
| `POST /access/v1/evaluation` | `adl.access_evaluation` |
| `POST /access/v1/evaluations` | `adl.access_evaluations` |
| `POST /access/v1/search/subject` | `adl.search_subject` |
| `POST /access/v1/search/resource` | `adl.search_resource` |
| `POST /access/v1/search/action` | `adl.search_action` |

Each AuthZEN route has its own typed logger method, so `event_name` follows
the endpoint that handled the decision rather than being inferred from the
shape of the request.

The Access API has two implementations, on separate ports:

- the directory (`internal/eds/pkg/directory/v3/access.go`) answers from its
  relationship graph, and serves all five routes;
- the authorizer (`daemon/authorizer/impl/access.go`) answers from the policy
  engine, and serves the two evaluation routes. The action names the rule:
  with a bundle rooted at `package authz`, an `action.name` of
  `request_laadpaal` evaluates `data.authz.request_laadpaal`. Under a nested
  package the action carries the remainder, e.g. `laadpalen.request_laadpaal`
  for `package authz.laadpalen`. The searches return `Unimplemented`, because
  a Rego rule answers "may this subject do this?" and offers no way to
  enumerate the subjects, resources or actions it would admit.

### Selecting a policy

One instance serves as many policies as its bundle carries. AuthZEN has no
policy field, so a request selects one through the context:

```json
{"context": {"doelbinding": "laadpalen"}}
```

which evaluates `data.doelbinding.laadpalen.<action>`. Selectable policies
live under the `doelbinding` prefix and nowhere else, so a request cannot
reach a library package by naming it, and an unknown doelbinding is an error
rather than a policy chosen on the caller's behalf. This follows OpenFTV,
whose OPA PDP routes each request to `data.doelbinding.<x>` the same way.

A request that selects nothing falls back to `opa.policy_root`:

```yaml
opa:
  policy_root: authz
```

which only needs setting when the bundle carries more than one package root,
typically a decision package alongside library packages. With a single root
there is nothing to disambiguate and it can stay empty. With several roots
and no setting, an unselective request is refused rather than bound to
whichever package the policy store happened to list first — that order is not
stable across restarts, and an authorization endpoint that silently changes
which policy it evaluates is worse than one that errors.

The Access API is the authorizer's only decision-making endpoint, so every
decision is in scope for ADL by construction.

Because the logger is shared with the directory, which has no OPA runtime,
it is not an OPA plugin. The `adl_decision_logger` plugin name is still
registered (as a no-op) so that OPA accepts the config block below.

## Configuration

```yaml
opa:
  config:
    plugins:
      adl_decision_logger:
        enabled: true
        output: 'stdout,otlp'         # comma-separated: stdout, otlp, or both. Defaults to both when unset. Settable via ${LOG_OUTPUT}.
        otlp:
          endpoint: 'localhost:4317'  # otlp/gRPC collector address, e.g. a Grafana Alloy receiver
          insecure: true              # disable TLS - typical for a same-cluster/sidecar collector
        resource:                     # producer identity, emitted as the record's `resource` field
          service.name: eamerald
        resource_context:             # keys read from an authorizer resource context to fill the AuthZEN resource
          type_key: 'object_type'
          id_key: 'object_id'
```

## Output

- `stdout` writes each record as a single JSON line, fields at the top
  level (not wrapped in any envelope), so a log-shipping agent can parse
  `trace_id`/`event_name`/`status`/`body` directly.
- `otlp` sends the same record (its JSON marshaled into the OTLP log
  record's body) via the OTLP/gRPC log export protocol, with `trace_id`/
  `span_id` carried as native OTel trace correlation fields rather than
  needing to be parsed out of the body. Export is asynchronous and batched,
  and buffered records are flushed on shutdown.

If `otlp` is selected but `otlp.endpoint` is empty, or the exporter fails to
initialize, otlp output is skipped for that run (logged as an error) —
stdout output, if also selected, is unaffected.

The OTLP export carries the `resource` map as OTLP resource attributes as
well as inside the record body, so a collector can label and route records
per PDP without parsing them. `service.name` defaults to `eamerald` when the
deployment did not set it, because collectors key off it — Loki turns it into
the stream's `service_name` label, and an unset one lands every PDP in the
same `unknown_service` stream.

### Shipping records to Loki

`k8s/observability` installs Loki, Grafana Alloy and Grafana for a
development cluster. Alloy receives the records over OTLP and forwards them
to Loki's native OTLP endpoint; Grafana comes with Loki provisioned:

```sh
make k8s-observability-install   # installs the stack and points eamerald at Alloy
make k8s-grafana                 # http://localhost:3000, admin/admin
```

Keep `stdout` on alongside `otlp`. The OTLP exporter batches, so a collector
outage loses whatever is still queued, and the pod log stays the durable
trail the spec asks for. Alloy scrapes pod logs too, and drops the ADL lines
from that path so a decision is not stored twice — the OTLP copy is the one
with its labels and trace correlation intact.

In Grafana's Explore, ADL records are their own stream, with the `resource`
entries and the trace IDs queryable as structured metadata:

```logql
{service_name="eamerald"}                                              # every record
{service_name="eamerald"} | json | body_adl_core_response_decision="false"  # denials
{service_name="eamerald"} | instance_id="eamerald-5ff55547f-6rp5c"     # one PDP
```

## Field mapping

| ADL field | Source |
|---|---|
| `trace_id` / `parent_span_id` | Parsed from the incoming W3C `traceparent`. Over REST this relies on the gateway forwarding the header — see `TraceContextHeaders` in `daemon/service/builder/defaults.go`. |
| `span_id` | Freshly minted per decision. |
| `event_name` | The endpoint that handled the decision (see Scope). |
| `timestamp` | Milliseconds since the Unix epoch, UTC. |
| `status` | `Ok` when the PDP reached a decision. A denial is still `Ok`: denial is a valid outcome, and `Error` is reserved for the PDP failing to evaluate at all. |
| `attributes` | Always `{}` — Level 1 carries no source references. |
| `resource` | The configured `resource` map, plus an `instance_id` identifying the PDP that decided (the host/pod name, or a random id). The spec requires this whenever records are aggregated outside the producing organisation, and it is what keeps the records of replicas, and of separately deployed PDPs, distinguishable. Set `instance_id` in config to override it. |
| `body["adl.core.request"]` | The AuthZEN request. Present for both `Ok` and `Error` records, showing what was attempted. |
| `body["adl.core.response"]` | The AuthZEN response. Present only when `status` is `Ok` — omitted for `Error`, since no decision was reached. |

### Authorizer specifics

The authorizer's Access API records the request the caller sent, with two
adjustments:

- **Subject** is replaced by the directory user the identity resolved to, not
  the caller's own properties: `subject.properties.jwt` is how a caller hands
  the PDP a bearer token, and that must never reach the log.
- **Context** gains `policy_path`, holding the package the action was
  resolved against - the request names a doelbinding, not a package, and
  without it a record can't be tied back to the rule that decided.

## Known deviations

- **Durability.** The spec's guidance is that a record should reach durable
  storage before the decision is returned to the PEP. OTLP export is
  asynchronous and batched, and the stdout write is not `fsync`'d, so neither
  output meets that on its own; the stdout trail plus the collector's own
  durability is what the deployment relies on. A logging failure does fail
  the request, so no decision is returned without at least an attempted
  record.
- **No distributed tracing.** This package uses OpenTelemetry's *logs* SDK;
  it creates and exports no spans. It reads and mints W3C-shaped trace IDs so
  that a genuine incoming `traceparent` is preserved, and carries them as
  correlation fields, but eamerald does not yet propagate trace context on
  its own outgoing calls (e.g. to PIPs), which the spec requires of every
  component in the evaluation.
- **Level 1 only.** `adl.core.policies`, `adl.core.information` and
  `adl.core.configuration` are not recorded; those begin at Level 2.
