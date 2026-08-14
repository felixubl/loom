# loom — project context

`docs/loom-v0-spec.md` is normative. Read it before changing semantics. The
section numbers cited in source comments and test names refer to it.

## What this is

loom is a semantic kernel: one composition operation, `F(X)`. Some applications
reduce, the rest stay symbolic, and the symbolic ones are the graph. There is no
primitive edge, relation, or triple.

Persistence adds exactly four things: asserted values, claims, `holds`, `match`.

## Layers, and the rule for each

- **Term** (atom, lambda, application) is pure and never mutates anything.
- **World snapshot** (`holds`, `match`) reads one consistent snapshot.
- **Transaction** (`assert`, `retract`) is the only thing that changes state.

Never let a write happen during evaluation. That separation is normative (§33).

## Before adding anything

Answer §44's six questions first. Can it be ordinary application? Can it be a
library function? Is it only syntax? Is it only an index or optimizer? Does it
need to read the persistent world? Does it need to change it? Only genuinely
irreducible concepts belong below the corresponding boundary.

v0 is deliberately small. Arithmetic, `if`, lists, traversal, reactivity,
temporal validity, query planning, schemas, properties, and the Look language
are all out of scope (§2, §41). Do not add them to v0 because they seem needed;
they belong to later milestones (§42, §43).

## Code

Standard Go toolchain, standard library only. No third-party dependencies.

```bash
go test ./...
go test -race ./...
go vet ./...
gofmt -l .
```

Layout, one file per concern:

| File | Owns |
| --- | --- |
| `value.go` | the canonical hash-consed value arena and the `Store` |
| `term.go` | the term AST |
| `parse.go` | surface syntax for terms and patterns, store-free for terms |
| `eval.go` | values, closures, application, primitives |
| `world.go` | snapshots, claims, `holds` |
| `match.go` | patterns and structural matching |
| `tx.go` | transactions |
| `errors.go` | the §34 error codes |
| `limits.go` | the §35 resource policy |

Invariants worth keeping:

- `ValueID` is 1-based; zero means "no canonical identity". A `Neutral` with
  `ID == 0` is a legal value that is not persistable.
- Lock order is arena before journal. Nothing takes the journal lock and then
  the arena lock.
- A `Tx` and any `World` taken from it belong to one goroutine. The `Store` and
  worlds from `Store.World` are safe to share.
- Rendering a value must stay iterative. Values nest as deeply as evaluation
  allows, and printing has no resource limit of its own.

`conformance_test.go` implements §40 case by case. A change that makes one of
those fail is a change to the specification, not to the code.
