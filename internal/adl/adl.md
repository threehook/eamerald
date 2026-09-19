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
| `POST /api/v2/authz/is` | `adl.access_evaluation` or `adl.access_evaluations` |

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

### One policy per PDP

An access evaluation request carries no policy selector, because AuthZEN
assumes a policy decision point evaluates one policy. So does this
implementation: the deployment decides which policy an instance serves, and
serving several means running several instances — one PDP each, as OpenFTV
does with one PDP process per bundle.

`opa.policy_root` names that policy. It only needs setting when the loaded
bundle carries more than one package root, which is typically a decision
package alongside library packages:

```yaml
opa:
  policy_root: authz
```

With a single root there is nothing to disambiguate and it can stay empty.
With several roots and no setting, the Access API refuses the request rather
than binding to whichever package the policy store happened to list first —
that choice is not stable across restarts, and an authorization endpoint that
silently changes which policy it evaluates is worse than one that errors.

This is scoped to the Access API. `Is()` and `Query()` take a policy path per
request and keep working against a multi-policy bundle, so a bundle that is
ambiguous for the Access API only produces a startup warning, not a startup
failure.

`Is()` is not an AuthZEN endpoint, so it is recorded under the AuthZEN model
its information model corresponds to, as the spec directs for OPA-style
direct calls: one requested decision is an Access Evaluation, a list of them
is an Access Evaluations call against a single subject and resource.

### What is not logged

The authorizer's `Query()`, `DecisionTree()` and `Compile()` produce no
records.

This is not because they are not AuthZEN endpoints. The standard's scope is
"any authorization decision representable in the AuthZEN information model,
regardless of the wire protocol by which the decision is delivered" — so the
test is the shape of the decision, not the route it arrived on. `Compile()`
returns residual queries from partial evaluation, which is genuinely not
representable. `Query()` and `DecisionTree()` return arbitrary Rego results,
of which *some* are decisions and most are introspection, and nothing in the
request distinguishes the two.

So a decision that must be logged has to be asked for somewhere its shape is
known. That is what the authorizer's Access API is for: a rule returning
`{"decision": bool, "context": {...}}` is an Access Evaluation, and asking
for it over `/access/v1/evaluation` rather than `Query()` is what brings it
into scope. A policy decision still served over `Query()` is not logged and
is not compliant.

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

## Field mapping

| ADL field | Source |
|---|---|
| `trace_id` / `parent_span_id` | Parsed from the incoming W3C `traceparent`. Over REST this relies on the gateway forwarding the header — see `TraceContextHeaders` in `daemon/service/builder/defaults.go`. |
| `span_id` | Freshly minted per decision. |
| `event_name` | The endpoint that handled the decision (see Scope). |
| `timestamp` | Milliseconds since the Unix epoch, UTC. |
| `status` | `Ok` when the PDP reached a decision. A denial is still `Ok`: denial is a valid outcome, and `Error` is reserved for the PDP failing to evaluate at all. |
| `attributes` | Always `{}` — Level 1 carries no source references. |
| `resource` | The configured `resource` map, plus an `instance_id` identifying the PDP that decided (the host/pod name, or a random id). The spec requires this whenever records are aggregated outside the producing organisation, and with one PDP per policy it is what keeps their records — and those of replicas of one PDP — distinguishable. Set `instance_id` in config to override it. |
| `body["adl.core.request"]` | The AuthZEN request. Present for both `Ok` and `Error` records, showing what was attempted. |
| `body["adl.core.response"]` | The AuthZEN response. Present only when `status` is `Ok` — omitted for `Error`, since no decision was reached. |

### Authorizer specifics

`Is()` takes Topaz-shaped input, which is translated to the AuthZEN model in
`daemon/authorizer/impl/adl.go`:

- **Subject** is the directory user the identity resolved to, so its type and
  id are what the policy was evaluated against. For `IDENTITY_TYPE_JWT` the
  identity value is the bearer token itself, which is never written to the
  log: if a record is produced before resolution completed, the subject id is
  left empty rather than leaking a live credential.
- **Resource** type and id are lifted out of the resource context under the
  `resource_context` keys, since AuthZEN requires a resource type and a Topaz
  resource context has no fixed schema. The whole context is kept as the
  resource properties either way.
- **Context** carries `policy_path`. AuthZEN has no field for it — the API
  assumes one PDP evaluates one policy — but Topaz takes a policy path per
  request, so without it a record cannot be tied back to the rule that
  decided.

The authorizer's Access API records the request the caller sent, with the
same two adjustments: `policy_path` is added to the context (holding the
package root the action was resolved against), and the subject is replaced by
what the identity resolved to. Subject *properties* are dropped rather than
logged, because `subject.properties.jwt` is how a caller hands the PDP a
bearer token.

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
