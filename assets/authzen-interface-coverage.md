# AuthZEN Access API test coverage

Each template below registers assertion files that run against a real
eamerald instance as part of `make test`, exercising the AuthZEN Access
API's five interfaces: `evaluation`, `evaluations`, `subject_search`,
`resource_search`, and `action_search`.

| Template | `evaluation` | `evaluations` | `subject_search` | `resource_search` | `action_search` |
|---|---|---|---|---|---|
| gdrive | ✓ | | ✓ | ✓ | |
| api-auth | ✓ | ✓ | | | |
| api-gateway | ✓ | | | | ✓ |
| multi-tenant | ✓ | ✓ | | ✓ | |
| slack | ✓ | | | | ✓ |
| simple-rbac | ✓ | ✓ | | | |
| github | ✓ | | | | ✓ |
| acmecorp | ✓ | | | ✓ | |
| citadel | ✓ | | ✓ | | |
| peoplefinder | ✓ | | | | |
| todo | ✓ | | ✓ | | |
