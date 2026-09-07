# Security policy

## Supported versions

refract follows semantic versioning. Security fixes are made on the latest
minor release of the current major line; older lines are not patched.

| Module | Supported |
|---|---|
| `github.com/timzifer/refract` | `v1.x` |
| `github.com/timzifer/refract/backend/gg` | `v1.x` |
| `github.com/timzifer/refract/backend/window` | `v1.x` |
| `github.com/timzifer/refract/backend/gg/gpu` | `v0.x` (pre-1.0, latest only) |
| `github.com/timzifer/refract/arrow/v18` | `v18.x` |

## Reporting a vulnerability

Please **do not open a public issue** for a security problem.

Report it privately through GitHub's
[private vulnerability reporting](https://github.com/timzifer/refract/security/advisories/new)
form. That opens a draft advisory only the maintainers and you can see.

What helps most: the affected module and version, a description of the impact,
and the smallest input or program that reproduces it.

You can expect an acknowledgement within seven days and a status update at
least every fourteen days until the report is resolved. When a fix ships, the
advisory is published with credit to the reporter unless you ask otherwise.

## Scope

refract renders charts from data a program hands it. The things worth reporting
are the ones where that data crosses a trust boundary: a malformed or hostile
data set that causes a panic in a decoder, an out-of-bounds access, unbounded
memory growth, or output that escapes its encoding — SVG or PDF that breaks out
of the document it is written into. Rendering that is merely ugly or wrong is a
normal bug; open an issue for it.
