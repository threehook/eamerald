# Authorizer endpoint possibilities

The authorizer currently serves the AuthZEN Access API (`/access/v1/*`) for
every decision. Three further capabilities are on the table for later,
each answering a different kind of question than "evaluate this one
decision." This is a starting point for that conversation, not a plan -
each needs a real design pass and its own tests before it happens.

## Ad-hoc / debugging queries

**The question:** "What does this rule evaluate to, right now, against live
data?" - for someone actively developing or troubleshooting a policy, or a
console feature that lets an operator try a rule interactively, including
timing and evaluation-trace output.

**What it would take:** its own endpoint, deliberately outside AuthZEN's
scope (the spec has no ad-hoc-query concept, so this isn't an AuthZEN
endpoint and shouldn't pretend to be), and explicitly excluded from ADL
since its result has no fixed decision shape. Worth scoping first against
what `opa eval` against a local bundle copy already covers, so the endpoint
only adds what that can't.

## Bulk decisions under one policy

**The question:** "For this subject and resource, give me every named
decision under this policy area in one round trip" - e.g. a UI that wants
`allowed`/`visible`/`enabled` for several routes at once, without firing one
request per route.

`/access/v1/evaluations` already batches many *named* checks in a single
call - the gap is discovery: a client still has to already know every action
name up front. Closing that means either (a) a policy-authored manifest
listing "these are the actions available for this resource type," so a
client can ask for "everything for X" without hardcoding names, or (b)
treating that as out of scope and expecting clients to know their own
action set. Either is a real design decision, not a mechanical rebuild.

## Query pushdown / partial evaluation

**The question:** "Which rows can this user see" - answered as a filter
expression a database can apply directly, instead of checking access one
row at a time. OPA's partial evaluation already does this: given a query
with some inputs marked unknown, it returns the residual condition.

This is architecturally distinct from a decision or a list of results, so it
would live as its own API surface alongside the Access API, not an
extension of it. Most valuable paired with a concrete use case - e.g.
authorizing a list/search endpoint against a real database - since a
residual expression is only useful to a caller that can consume one.
