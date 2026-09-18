# ADL Decision Logger plugin

Writes one record per `Is()` call, conforming to the Logius ADL 1.0 Level 1
record shape:
https://gitdocumentatie.logius.nl/publicatie/ftv/adl/1.0.0/

Per §3.3.9, every evaluated request must produce exactly one record - this
includes requests the PDP failed to evaluate, not just ones it decided on.

Supports two outputs, independently or together (see `output` below):
a JSON line to stdout, meant to be scraped by a log-shipping agent (e.g.
Grafana Alloy) and forwarded to Loki; and/or a direct OTLP log export,
which the ADL spec (§3.1) recommends as the transport between the
application and the log.

Plugin configuration structure

```
opa:
  config:

    # plugins section
    plugins:
      adl_decision_logger:
        enabled: true
        output: 'stdout,otlp'        # comma-separated: stdout, otlp, or both. Defaults to both when unset. Settable via ${LOG_OUTPUT}.
        otlp:
          endpoint: 'localhost:4317'  # otlp/gRPC collector address, e.g. a Grafana Alloy receiver
          insecure: true               # disable TLS - typical for a same-cluster/sidecar collector
```

## Output

- `stdout` writes each record as a single JSON line, fields at the top
  level (not wrapped in any envelope), so a log-shipping agent can parse
  `trace_id`/`event_name`/`status`/`body` directly.
- `otlp` sends the same record (its JSON marshaled into the OTLP log
  record's body) via the OTLP/gRPC log export protocol, with `trace_id`/
  `span_id` carried as native OTel trace correlation fields rather than
  needing to be parsed out of the body. Export is asynchronous and
  batched: an unreachable collector cannot slow down or fail an `Is()`
  call, and buffered records are flushed on plugin `Stop`.

If `otlp` is selected but `otlp.endpoint` is empty, or the exporter fails
to initialize, otlp output is skipped for that run (logged as an error) -
stdout output, if also selected, is unaffected.

## Field mapping

| ADL field | Source |
|---|---|
| `trace_id` / `parent_span_id` | Parsed from the incoming W3C `traceparent` gRPC metadata header, if present. |
| `span_id` | Freshly minted per decision. |
| `event_name` | `adl.access_evaluation` when the `Is()` call requests exactly one decision; `adl.access_evaluations` when it requests more than one. |
| `status` | `Ok` when the PDP reached a decision. A denial is still `Ok` (spec §3.3.6: denial is a valid outcome, `Error` is only for the PDP failing to evaluate at all). `Error` when the PDP could not produce a decision: request validation, identity resolution, runtime lookup, query preparation, evaluation, or undefined results. |
| `attributes` | Always `{}` (Level 1 carries no source references). |
| `body["adl.core.request"]` | An AuthZEN `EvaluationRequest`/`EvaluationsRequest`, built from the `Is()` request's identity/resource context and requested decision names. Present for both `Ok` and `Error` records, showing what was attempted. |
| `body["adl.core.response"]` | An AuthZEN `EvaluationResponse`/`EvaluationsResponse` carrying the decision outcomes. Present only when `status` is `Ok` — omitted for `Error`, per spec §3.3.8, since no decision was reached. |
| `resource` (producer identity) | Omitted — not required at Level 1. |

## Scope

This plugin logs the `Is()` RPC only, both successful and failed
evaluations.

## Not included

This plugin uses OpenTelemetry's *logs* SDK to export records via OTLP - it
does not do distributed *tracing*: no TracerProvider, no spans created or
exported. It only reads/mints W3C-shaped trace IDs so that a genuine
incoming `traceparent` is preserved when one exists, and carries them as
correlation fields on the records it emits. Real distributed tracing (an
actual TracerProvider and span export, spanning every service involved in
a request) is a separate, larger effort.
