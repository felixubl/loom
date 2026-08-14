<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/wordmark-dark.svg">
  <img src="docs/wordmark-light.svg" alt="loom" width="216">
</picture>

A semantic kernel in which computation and persistent graph structure are the
same thing.

```
Everything evaluates to a value.
Application is universal.
Some applications reduce.
The rest are symbolic graph structure.
```

## The idea

loom has one composition operation, `F(X)`. It does not mean "call a function".
It means apply one value to another value.

Some applications reduce:

```
(x => x)(Alice)
→ Alice
```

Some have nothing to reduce with, and stay symbolic:

```
knows(Alice)(Bob)
→ knows(Alice)(Bob)
```

That second kind is the graph. There is no primitive edge, no relation type, no
`(subject, predicate, object)` triple built into the kernel. A triple is just an
application of an application, and the relation in it is an ordinary value.

Constructing a value and claiming it is true are different operations:

```
knows(Alice)(Bob)          builds a value
holds(knows(Alice)(Bob))   asks whether it is asserted
assert knows(Alice)(Bob)   asserts it
```

Persistence adds four things to the calculus and nothing else: asserted values,
claims, `holds`, and `match`.

## The language

A whole Loom program is definitions, commands, and expressions.

```
# A function whose result is symbolic.
friend = x => y => knows(x)(y)

transaction {
    assert friend(Alice)(Bob)
    assert friend(Bob)(Charlie)
}

answer = holds(friend(Alice)(Bob))
match knows(?who)(?whom)
```

An identifier is a variable when an enclosing lambda binds it, and a reference
otherwise. A reference nothing defines evaluates to the atom of its own name.
That one rule is why computation and graph construction need no separate
notation:

```
friend                 → x => y => knows(x)(y)
friend(Alice)          → y => knows(Alice)(y)
friend(Alice)(Bob)     → knows(Alice)(Bob)
```

Both lambdas reduce, then reduction stops because `knows` is inert. A function
just built a persistable fact out of ordinary application.

There are no operators, no `if`, no blocks, no loops, and no `let`. No
arithmetic either: write `add(2)(3)`, not `2 + 3`. Those are all future surface
growth, and none of them requires the semantic kernel to grow.

[`docs/loom-surface-v0.md`](docs/loom-surface-v0.md) describes the syntax in
full.

## The REPL

```
$ go run ./cmd/loom
loom> x => x
x => x
loom> (x => x)(Alice)
Alice
loom> knows(Alice)
knows(Alice)
loom> knows(Alice)(Bob)
knows(Alice)(Bob)
loom> assert knows(Alice)(Bob)
claim:1
loom> holds(knows(Alice)(Bob))
true
loom> match knows(Alice)(?x)
x = Bob
```

```
loom program.loom      run a program, then exit
loom -i program.loom   run a program, then continue interactively
```

See [`examples/friends.loom`](examples/friends.loom).

## Status

This is v0: the smallest kernel the rest can be built on top of, and nothing
more. It is implemented and every conformance case in the specification passes.

Built:

- atoms, variables, references, lambdas, call-by-value application
- closures, with lexical capture by environment
- neutral (symbolic) applications
- structural equality by hash-consed canonical interning
- immutable world snapshots
- claims, `assert`, `retract`, `holds`
- structural pattern matching
- transactions with staged visibility
- resource limits on steps, depth, value size and result count
- a source language with definitions and commands, a script runner, and a REPL

Deliberately not built: arithmetic, `if`, lists, sets, `map`/`filter`/`fold`,
traversal, reactivity, temporal validity, query planning, schemas, properties,
named edges, multi-argument calls, a standard library, and the Look query
language. Those belong above this layer, and the point of v0 is that adding them
becomes a layering exercise rather than a semantic redesign.

## Using it

```
go get github.com/felixubl/loom
```

```go
store := loom.New()

// Writes are transaction commands, never ordinary expressions.
tx := store.Begin()
fact, err := loom.Parse("knows(Alice)(Bob)")
if err != nil {
	log.Fatal(err)
}
if _, err := tx.Assert(fact); err != nil {
	log.Fatal(err)
}
if err := tx.Commit(); err != nil {
	log.Fatal(err)
}

// Reads run against an immutable snapshot.
world := store.World()
question, err := loom.Parse("holds(knows(Alice)(Bob))")
if err != nil {
	log.Fatal(err)
}
answer, err := world.Eval(question)
if err != nil {
	log.Fatal(err)
}
id, err := loom.Persist(answer)
if err != nil {
	log.Fatal(err)
}
fmt.Println(store.Source(id))    // true

// Patterns query held values structurally.
pattern, err := store.ParsePattern("knows(?who)(?whom)")
if err != nil {
	log.Fatal(err)
}
rows, err := world.Match(pattern)
if err != nil {
	log.Fatal(err)
}
for _, row := range rows {
	fmt.Println(store.Source(row["who"]), "knows", store.Source(row["whom"]))
}
```

## Three layers

The kernel keeps three things apart, and the separation is normative.

| Layer | Contains | Rule |
| --- | --- | --- |
| Term | atom, lambda, application | pure, never mutates |
| World snapshot | `holds`, `match` | reads one consistent snapshot |
| Transaction | `assert`, `retract` | the only way anything changes |

Because writes are commands rather than expressions, they cannot vanish because
a value went unused, run twice because a thunk was forced twice, reorder because
an optimizer moved pure code, or fire by accident while something inspects the
graph.

## Specification

[`docs/loom-v0-spec.md`](docs/loom-v0-spec.md) is normative. The section numbers
in it are cited throughout the source, and `conformance_test.go` implements
§40 case by case.

Before adding anything to loom, the specification asks six questions (§44). Can
it be expressed with ordinary application? Can it be a library function? Is it
only convenient syntax? Is it merely an index or an optimizer? Does it need to
observe the persistent world? Does it need to change it? Only genuinely
irreducible concepts belong below the corresponding boundary.

The preference is minimal semantics, rich derivation, aggressive optimization.

## Development

```bash
go test ./...
go test -race ./...
go vet ./...
gofmt -l .
```

The package has no dependencies outside the Go standard library, and is meant to
keep it that way.

## License

Apache License 2.0. See [LICENSE](LICENSE).
