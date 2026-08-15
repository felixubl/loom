# Roadmap

Each version adds **one** new semantic capability and proves it composes with
the layer beneath it. Do not jump from v0 to "full Loom".

## Where we are

| Version | Adds | State |
| --- | --- | --- |
| v0 | applicative kernel | done |
| v0.1 | freeze semantics behind a conformance corpus | done |
| v0.2 | core values and functions | next |
| v0.3 | collections | |
| v0.4 | control without special syntax | |
| v0.5 | general recursive programming | |
| v0.6 | query core | |
| v0.7 | query planning boundary | |
| v0.8 | time and provenance | |
| v0.9 | reactivity | |
| 1.0 | useful without Look | |

## v0 — applicative kernel

Atoms, lambdas, unary application, neutral applications, lexical closures,
top-level recursive definitions, canonical persistable applications, claims,
`holds`, `match`, transactions, exact retraction, immutable snapshots.

## v0.1 — freeze semantics

Before adding any feature, hammer v0. The goal is that **two independent
implementations agree byte-for-byte on canonical persisted values and
result-for-result on evaluation**.

Covered: recursion, mutual recursion, shadowing, neutral application nesting,
structural equality, duplicate claims, snapshot consistency, transaction
visibility, pattern captures, persisted-value round trips.

The corpus is `conformance/corpus.json`, deliberately data rather than Go tests
so another implementation can run it. The encoding it pins is specified in
[`loom-canonical-form.md`](loom-canonical-form.md).

## v0.2 — core values

Efficient atoms for what users actually need: integer, float, text, boolean,
none. Then expose functions as ordinary Loom values:

```
add(2)(3)
equal(A)(B)
not(true)
```

Still no `+`, `==`, or `if` syntax. The test is whether the standard functions
go through exactly the same `F(X)` machinery as user lambdas.

## v0.3 — collections

The first major addition. One minimal collection abstraction, likely an
immutable sequence and/or set, with `map`, `filter`, `fold`, `count`, `any`,
`all`, `one`. Derive as much as possible from `fold`. This is what turns match
results into something pleasant to program with.

## v0.4 — control without special syntax

Booleans and conditional evaluation. **The hardest language-design problem after
v0.** The kernel is call by value, so a naive

```
if(condition)(A)(B)
```

evaluates both branches before `if` sees them. That is wrong for recursion,
expensive branches, and any future effects. Solve one of: laziness and thunks,
a lazy primitive application form, or explicit delayed values. Solve it before
adding pretty `if` syntax.

## v0.5 — general recursive programming

Prove Loom is genuinely useful as a programming language: factorial, fibonacci,
list length, map, tree traversal, transitive closure. Not because factorial
matters, but because these exercise recursion, higher-order functions, symbolic
values, and collections together.

## v0.6 — query core

Make persistence compositional. Match results become ordinary values:

```
people = match(person(?x))
active = filter(x => holds(active(x)))(people)
```

Work out captures, joins, conjunction, negation, wildcards, projection, and a
deterministic result representation. This is the semantic foundation for Look.

## v0.7 — query planning boundary

Define explicitly which Loom expressions compile into indexed database queries:

```
Planable Loom ⊂ Loom
```

Never promise that arbitrary Loom programs are query plans. Start with
structural patterns, conjunction, equality, indexed predicates, simple filters,
and joins. Then benchmark them.

## v0.8 — time and provenance

Only now bring back immutable transaction history, snapshots, claim provenance,
`since`, and possibly `valid_until`. Keep it firmly in the persistence layer.
Do not contaminate application semantics with time.

## v0.9 — reactivity

Evaluation against a world reports what persistent information it observed:

```
evaluate(expr, W) → { value, dependencies, valid_until }
```

Then build cache invalidation on that. This is the clean version of the reactive
behaviour legacy loom arrived at the messy way.

## 1.0 — useful without Look

Functions, recursion, collections, booleans, numbers, strings, persistent
symbolic values, `holds`, `match`, transactions, modules, imports, basic error
handling, resource limits, canonical serialization.

The surface language should still be small:

```
friend = x => y => knows(x)(y)
friends = x =>
    match(knows(x)(?y))
```

Operators may exist by then, but they compile to ordinary core functions.

## Then Look

Resist working seriously on Look before the query core is solid. Look 0 is
almost entirely a compiler into Loom's planable query fragment:

| Look | Loom |
| --- | --- |
| `[Alice] >[knows] [Bob] ?` | `holds(knows(Alice)(Bob))` |
| `[Alice] >[knows]` | `match(knows(Alice)(?x))` |
| `(? >[knows] [Bob])` | `match(knows(?x)(Bob))` |

Then reintroduce the parts of the old Look that still feel excellent: `>[r]`,
`<[r]`, `<>[r]`, `and`, `or`, `not`, `order by`, `take`, `group by`, `!`, `?`,
`-`. Every one has to answer: **what Loom meaning does this compile to?** If
there is no clean answer, the feature belongs in the planner or runtime, or
Loom's core semantics are missing something.

## Then self-hosting

Only once Loom is pleasant enough for nontrivial software:

```
standard library → Look parser/compiler → query planner
                 → Loom parser → Loom compiler
```

Eventually the Loom compiler is written in Loom, and Go becomes a runtime and
bootstrap rather than the place the language lives.

## The classification rule

More important than the version numbers. Every proposed feature is classified as
exactly one of:

```
kernel semantic
core function
surface sugar
query optimization
runtime optimization
persistence capability
```

**The default answer is not kernel.**

- `sum` → can it be `fold(add)(0)`? Then it is a core function.
- `+` → surface sugar for `add(x)(y)`.
- `>[knows]` → Look syntax for a pattern.
- a `(relation, subject, object)` index → a storage optimization over
  application spines.
