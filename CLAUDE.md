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
| `parse.go` | surface syntax for terms, patterns and statements |
| `resolve.go` | the phase between parsing and evaluation |
| `eval.go` | values, closures, application, primitives, `Display` |
| `world.go` | snapshots, claims, `holds` |
| `match.go` | patterns and structural matching |
| `tx.go` | transactions |
| `program.go` | definitions, commands, `Session` |
| `render.go` | the one result rendering the REPL and corpus share |
| `text.go` | the specified escaping for canonical and surface text |
| `conformance/` | the corpus that freezes v0, and its runner |
| `errors.go` | the §34 error codes |
| `limits.go` | the §35 resource policy |
| `cmd/loom` | the REPL and script runner |

Two languages are in play and confusing them wastes time. Go is the *host*: it
implements the interpreter and a user never needs to know it exists. Loom is the
language in `docs/loom-surface-v0.md`. Surface syntax may grow without the
semantic kernel growing, and that is the point. `2 + 3` would lower to
`add(2)(3)`, a block to a lambda application. Never add meaning to the kernel to
serve syntax.

Invariants worth keeping:

- Parsing decides nothing about names. `Parse` emits `TermName`; `Resolve`
  turns each into `TermVar`, `TermDef` or `TermAtom` by the precedence
  parameter > definition > base binding > atom. Evaluating a `TermName` is an
  error. Do not move that decision back into the parser.
- The atom fallback is load-bearing: it is what lets `identity` resolve while
  `knows` stays inert in the same expression.
- Scoping is lexical and must stay that way. A `Closure` carries its `Env`, and
  applying one never consults the caller's environment. Top-level definitions
  are cells created before any body is resolved, which is what gives recursion
  and mutual recursion without dynamic lookup.
- An intrinsic's identity is the value, not its name. `holds = x => x` rebinds
  the name only. Compiled forms (Look, later) must hold the value from
  `Store.Intrinsic`, never re-look-up the text.
- A newline ends a statement that is COMPLETE there, and is ignored where a term
  is still required (after `=`, `=>`, `assert`, `retract`, `match`). An
  unparenthesized application's `(` may only continue on the same line. Without
  the first half, consecutive statements fuse and `(x => x)(Alice)` becomes an
  argument to the line above; without the second, a lambda body cannot sit on
  its own line. `SyntaxError.Incomplete` marks "ran out" so the REPL keeps
  reading.
- `match` is on the read side, not the write side. `Mutates` is the boundary,
  and it will matter for caching and reactivity.
- `ValueID` is 1-based; zero means "no canonical identity". A `Neutral` with
  `ID == 0` is a legal value that is not persistable.
- Lock order is arena before journal. Nothing takes the journal lock and then
  the arena lock.
- A `Tx` and any `World` taken from it belong to one goroutine. The `Store` and
  worlds from `Store.World` are safe to share.
- Rendering a value must stay iterative. Values nest as deeply as evaluation
  allows, and printing has no resource limit of its own.

`conformance/corpus.json` freezes v0 semantics. It is **data, not Go tests**, so
another implementation can run it; keep it that way. A change that makes a case
fail is a change to the specification, not to the code. `conformance_test.go`
beside the kernel covers what a linear program cannot express: snapshot
consistency and resource limits.

Before adding anything, read `docs/roadmap.md`. Every feature is classified as
exactly one of: kernel semantic, core function, surface sugar, query
optimization, runtime optimization, persistence capability. **The default answer
is not kernel.** `sum` is `fold(add)(0)`, `+` is sugar for `add(x)(y)`, a
`(relation, subject, object)` index is a storage optimization over application
spines. Versions add one capability at a time and in order.
