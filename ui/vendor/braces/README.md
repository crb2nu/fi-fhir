# Temporary braces security backport

This is a private `@fi-fhir/braces` fork, based on the MIT-licensed npm
[`braces@3.0.3`](https://www.npmjs.com/package/braces/v/3.0.3) runtime. It is
installed under the `braces` dependency name so existing micromatch consumers
use the repaired implementation. `3.0.3-fi-fhir.1` is our revision, not an
upstream release.

## Source and scope

[GHSA-vfj7-8cjw-p6xm](https://github.com/advisories/GHSA-vfj7-8cjw-p6xm)
(CVE-2026-93687) affects every published braces version as of 2026-10-03.
Deeply nested patterns can exhaust the stack despite the input-length limit.

`security.patch` contains only the runtime changes from the proposed upstream
[PR #72](https://github.com/micromatch/braces/pull/72), pinned at
`28d440b5dd449dbf1fe6f3506cf94ecca4d02660` against base
`e53730e6f935498326c72d768889ac194eedc0e0`. The PR is not merged or released.
We reviewed and applied those changes to the published 3.0.3 files, preserving
its unrelated quote and range semantics. The original license is retained.

The backport caps brace/parenthesis nesting at 100 in parsing and recursive
AST processing, honors smaller limits, and rejects cyclic AST parent chains.
Patterns over that limit intentionally fail early. Runtime files other than
`lib/{compile,constants,expand,parse,stringify}.js` are unmodified.

Reproduce by unpacking `npm pack braces@3.0.3`, retaining `index.js`, `lib/`
and `LICENSE`, then running `patch -p1 < security.patch` in that directory.
The local manifest contains only runtime dependencies; upstream development
dependencies and install scripts are not included.

## Installation and validation

`ui/package.json` declares `braces: file:vendor/braces` and the override
`braces: $braces`. Both are required: a relative file override alone can resolve
beneath a transitive consumer and produce a broken symlink with npm 10.
The Docker build copies `vendor/` before `npm ci`.

The normal UI test suite runs `src/lib/domain/dependencySecurity.test.ts`.
It resolves braces from micromatch's own installation and verifies depth
boundaries, attack-sized patterns, caller-supplied ASTs, parent cycles,
fractional limits and ordinary glob/expansion compatibility. The published
3.0.3 compatibility suite is also run against this backport before landing.

**Audit limitation:** npm recognizes the private fork under its own package
identity. A clean npm audit does not establish that the patch is correct and
will not automatically match future advisories filed against upstream braces.
Review upstream braces advisories during dependency maintenance, alongside
these behavior tests. No audit advisory is ignored or gate disabled.

Remove the fork, direct file dependency and override when an upstream release
covers this advisory and passes the same compatibility/security checks.
