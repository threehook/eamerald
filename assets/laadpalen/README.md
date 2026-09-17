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
- `ds-load/laadpalen.json` - the same data in Topaz's combined import format
- `gui/` - a small Vite/React app that calls the live authorizer directly and
  shows the decision + raw response
- `test_cases.json` / `test.sh` - a checklist of expected decisions and a
  script that checks them against a running deployment (see below)

The actual policy (`request_laadpaal`, in `package authz`) lives in the
separate `opa-policies` GitHub repo, which the chart's git policy-source
plugin polls automatically. This directory owns the schema and the example
data that policy runs against.

## Running it

Deploy first:

```
make laadpalen-deploy   # deploys topaz to k8s if not already running, then
                         # applies this manifest and data on top
```

`laadpalen-deploy` is additive - it layers this schema and data onto an
already-running topaz via the directory API, leaving the chart's own manifest
(`k8s/topaz/files/manifest.yaml`) as the generic starter model.

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
and sends each one as a real `POST /api/v2/authz/query` to a real, running
authorizer. It exercises the actual directory data, the actual git-sourced
Rego policy fetched from `opa-policies`, and the actual network path a real
caller would use - which is exactly why it lives here, run on demand, rather
than inside `make build lint test`: it depends on a live topaz deployment and
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
assets/laadpalen/test.sh https://some-other-host:8383
```
