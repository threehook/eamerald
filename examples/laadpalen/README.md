# laadpalen

An example authorization scenario for eamerald: a Dutch municipality's process
for approving a request for a laadpaal (EV charging station). It demonstrates
eamerald's full authorization stack together - a ReBAC directory (users,
departments, courses, diplomas, addresses), a git-sourced Rego policy, and an
AuthZEN-shaped decision (`{"decision": bool, "context": {"reason": string}}`).

## What's here

- `manifest.yaml` - the directory schema (department/course/diploma/adres types)
- `laadpalen_objects.json(l)` / `laadpalen_relations.json(l)` - the example data:
  5 users, 3 departments, 1 course, 3 diplomas, 4 fixed addresses
- `ds-load/laadpalen.json` - the same data in Eamerald's combined import format
- `gui/` - a small Vite/React app that calls the live authorizer directly and
  shows the decision + raw response
- `test_cases.json` / `test.sh` - a checklist of expected decisions and a
  script that checks them against a running deployment (see below)

The actual policy (`request_laadpaal`, in `package doelbinding.laadpalen`)
lives in the separate `opa-policies` GitHub repo, which the chart's git
policy-source plugin polls automatically. This directory owns the schema and
the example data that policy runs against.

## How the decision is requested

Over the AuthZEN Access Evaluation API, which the authorizer serves from the
policy engine:

```
POST https://localhost:8383/access/v1/evaluation

{
  "subject":  {"type": "user", "id": "jerry@example.com"},
  "action":   {"name": "request_laadpaal"},
  "resource": {"type": "adres", "properties": {"postcode": "1111BB", "huisnummer": 2}},
  "context":  {"doelbinding": "laadpalen"}
}
```

The action names the rule, and the doelbinding names the package it lives
in, so this evaluates `data.doelbinding.laadpalen.request_laadpaal`, and the
rule's `{"decision": ..., "context": ...}` return value *is* the response
body. Resource properties arrive flattened, which is why the policy reads
`input.resource.postcode` and not `input.resource.properties.postcode`.

This matters beyond tidiness: it's what every decision goes through the
Authorization Decision Log by - see `internal/adl/adl.md`.

## Running it

Deploy first, the same way as any other eamerald manifest - `MANIFEST` and
`DATA` name this one:

```
make k8s-deploy MANIFEST=examples/laadpalen/manifest.yaml \
  DATA="examples/laadpalen/laadpalen_objects.jsonl examples/laadpalen/laadpalen_relations.jsonl"
```

Every `make k8s-deploy` states its own `MANIFEST`, and applying one wipes the
deployment's existing directory data first, since old data may not be valid
under a different model.

`make k8s-deploy` deploys a hub (seeded with `MANIFEST`/`DATA`) and an edge
authorizer synced from it - see
[`docs/deployments/k8s-hub-edge.md`](../../docs/deployments/k8s-hub-edge.md).
The command doesn't return until the edge has completed its first sync from
the hub, so the deployment is ready to serve decisions by the time it exits.

From there, `laadpalen-gui` and `laadpalen-test` are two independent ways of
using that same deployment - neither depends on the other, and you can run
either, both, or neither:

```
make laadpalen-gui      # opens a browser frontend at http://localhost:5173
make laadpalen-test     # checks the decision matrix from the command line
```

## Playing with the GUI

Once it's running, you pick one of the five example people from a dropdown -
Rick, Morty, Beth, Jerry, Diane - each belonging to a different department
and holding a different diploma status. Then you either click one of the four
example addresses or type in your own postcode and huisnummer. Press "Toets
aanvraag" and you immediately see whether that person's request is approved
or denied, along with the real reason, in Dutch.

It's a way to explore the whole scenario yourself, by hand: see why Rick is
always turned down no matter the address (wrong department), why Jerry gets
approved at one address but not another (one already has a charging station,
another has no registered electric vehicle), and why only Diane can request
at the diplomatic address. Typing in a postcode that isn't one of the four
shows what happens when nothing matches, too.

## What kind of test is `laadpalen-test`?

A manual, local, end-to-end smoke check you run by hand against a running
deployment.

Concretely: `test.sh` reads `test_cases.json` (a fixed set of
user/postcode/huisnummer combinations and their expected `decision`/`reason`)
and sends each one as a real `POST /access/v1/evaluation` to a real, running
authorizer. It exercises the actual directory data, the actual git-sourced
Rego policy fetched from `opa-policies`, and the actual network path a real
caller would use - which is exactly why it lives here, run on demand, rather
than inside `make build lint test`: it depends on a live eamerald deployment and
on GitHub being reachable to sync the policy.

**How to use it:** run it yourself, by hand, after `make laadpalen-deploy`,
whenever you change the policy or the example data, to confirm the decision
matrix still behaves as expected:

```
$ make laadpalen-test
PASS  rick@example.com  1111AA/1   -> Niet geautoriseerd vanwege afdeling
...
10/10 passed
```

A `FAIL` line means something changed the actual decision for that case -
update either the fix or `test_cases.json`, depending on which one is now
wrong.

You can also point it at a different authorizer:

```
examples/laadpalen/test.sh https://some-other-host:8383
```
