# Authorization models

Eamerald supports the three common authorization models - ReBAC, RBAC, and
ABAC - and lets you combine them in a single decision. This page explains
what each one means in eamerald's own terms, with a working example for
each.

| Model | How eamerald implements it | Example |
|---|---|---|
| ReBAC (relationship-based) | Relations and permissions declared in `manifest.yaml`, evaluated by the built-in directory walking connected relations - no policy code | [`assets/acmecorp`](../assets/acmecorp) |
| RBAC (role-based) | The same relation walk as ReBAC, using relations named after roles (`owner`, `writer`, `member`, ...) | [`assets/simple-rbac`](../assets/simple-rbac) |
| ABAC (attribute-based) | A Rego policy that reads properties of the request itself (postcode, status, time, ...) rather than a stored relationship | [`assets/laadpalen`](../assets/laadpalen) |

## ReBAC - relationship-based access control

You declare object types, their relations, and permissions built from those
relations in `manifest.yaml`. A decision is made by following the
relations the directory already holds - group membership, an org chart,
document ownership - with no code to write.

`assets/acmecorp` models a management chain: `in_management_chain` is
defined as `manager | manager->identifier | manager->in_management_chain`,
so checking whether one employee is in another's chain walks the `manager`
relation until it finds them or runs out.

## RBAC - role-based access control

Eamerald has no separate role engine - a role is just a relation whose name
reads like one. `assets/simple-rbac` defines `owner`, `writer`, and
`reader` relations on a resource, and permissions such as `can_read` and
`can_write` as expressions over them. It runs through the exact same
relation walk as any other ReBAC model.

## ABAC - attribute-based access control

Some decisions depend on properties of the request itself rather than a
stored relationship - a postcode, a resource's status, the time of day.
Those go in a Rego policy: the policy reads the request's subject, action,
resource, and context, and returns the decision.

`assets/laadpalen` is the working example: whether someone may request an
EV charging station depends on the address's postcode and house number,
and whether that address already has a station - none of that is a
relationship, so the `request_laadpaal` policy checks it directly against
the request's resource properties.

## Combining them

A single decision can use both. `assets/laadpalen` does exactly this: it
checks a person's department and diploma through ReBAC relations, and the
address's attributes through the Rego policy, in the same evaluation.

## How this reaches the API

Every model is served through the same AuthZEN Access API `evaluation`
call. The difference shows up in the search endpoints
(`subject_search`, `resource_search`, `action_search`): a ReBAC or RBAC
model can answer those, because the directory can enumerate its own
relation graph. A Rego-based (ABAC) decision only answers yes/no for the
one request it was given, so those searches fall back to the directory
even for a model that also uses Rego.
