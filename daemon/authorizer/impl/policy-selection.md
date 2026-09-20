# Access API policy selection

How an AuthZEN Access API request resolves to the Rego rule that decides it.

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

## Selecting a policy

One instance serves as many policies as its bundle carries. AuthZEN has no
policy field, so a request selects one through the context:

```json
{"context": {"doelbinding": "laadpalen"}}
```

which evaluates `data.doelbinding.laadpalen.<action>`. Selectable policies
live under the `doelbinding` prefix and nowhere else, so a request cannot
reach a library package by naming it, and an unknown doelbinding is an error
rather than a policy chosen on the caller's behalf.

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

The package a request resolved against is recorded on every decision as
`context.policy_path` - see `internal/adl/adl.md`.
