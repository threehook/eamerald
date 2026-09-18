# ADL Decision Logger plugin

Writes one JSON line per `Is()` decision to stdout, conforming to the Logius
ADL 1.0 Level 1 record shape:
https://gitdocumentatie.logius.nl/publicatie/ftv/adl/1.0.0/

Intended to be scraped from container stdout by a log-shipping agent (e.g.
Grafana Alloy) and forwarded to Loki — this plugin does not write files or
ship logs itself.

Plugin configuration structure

```
opa:
  config:

    # plugins section
    plugins:
      adl_decision_logger:
        enabled: true
```

## Field mapping

| ADL field | Source |
|---|---|
| `trace_id` / `parent_span_id` | Parsed from the incoming W3C `traceparent` gRPC metadata header, if present. |
| `span_id` | Freshly minted per decision. |
| `event_name` | `adl.access_evaluation` when the `Is()` call requests exactly one decision; `adl.access_evaluations` when it requests more than one. |
| `status` | Always `Ok` — this plugin only logs decisions that were successfully evaluated (a denied decision is still `Ok`); evaluation failures never reach the logger since `Is()` returns an error before decisions exist. |
| `attributes` | Always `{}` (Level 1 carries no source references). |
| `body["adl.core.request"]` / `body["adl.core.response"]` | An AuthZEN `EvaluationRequest`/`EvaluationResponse` (or the batch `Evaluations...` form), built from the `Is()` request's identity/resource context and decision outcomes. |
| `resource` (producer identity) | Omitted — not required at Level 1. |

## Scope

Only the `Is()` RPC is covered. `DecisionTree`, `Query`, and `Compile` do not
emit decision logs today (via either this plugin or `eamerald_file_decision_logger`)
and are out of scope for this plugin too.

## Not included

Real distributed tracing (an OpenTelemetry SDK, a TracerProvider, span export)
is a separate, larger effort spanning every service involved in a request —
not something this plugin does. It only reads/mints W3C-shaped trace IDs so
that a genuine incoming `traceparent` is preserved when one exists, and its
own IDs stay forward-compatible with adding real tracing later.
