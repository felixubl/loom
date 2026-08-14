# Loom surface syntax v0

Status: descriptive. [`loom-v0-spec.md`](loom-v0-spec.md) is the normative
document and defines the semantics. This describes how a human writes them down.

Two different languages are involved and confusing them makes everything harder.
Go is the *host* language: it implements the interpreter and a user never needs
to know it exists. Loom is the language described here. Nothing below adds
meaning to the kernel. Every construct either denotes a term the kernel already
has, or is a command the kernel already defines (§28).

## Terms

```
term := atom | variable | reference | variable "=>" term | term "(" term ")"
```

Four things, and that is all:

```
Alice                atom
x => x               lambda
x => y => x          nested lambda
f(x)                 application
```

Application associates left, so `f(x)(y)` means `(f(x))(y)`. There is no
multi-argument call. `f(x, y)` may one day exist as sugar, and would mean
`f(x)(y)`.

Atoms are names, integers, or strings:

```
Alice   knows   task   42   "hello"   true   false
```

The kernel needs no separate categories for these (§5). The tag on a literal
exists only so the name `42` and the string `"42"` stay different atoms.

## Names

An identifier is a **variable** when an enclosing lambda binds it, and a
**reference** otherwise. A reference that nothing defines evaluates to the atom
of its own name.

That single rule is what lets computation and graph construction share one
syntax. In this program `identity` resolves to a function and `knows` stays
inert, with nothing distinguishing them at the call site:

```
identity = x => x

identity(Alice)        → Alice
knows(Alice)           → knows(Alice)
knows(Alice)(Bob)      → knows(Alice)(Bob)
```

Reduction simply runs out when it reaches something with no reduction behavior,
and what is left is graph structure.

## Definitions

```
definition := name "=" term
```

Definitions are top-level bindings. There is no `let`, and none is needed:
parameters already provide local names.

```
identity = x => x
constant = x => y => x
friend   = x => y => knows(x)(y)
```

A definition may refer to itself, because references resolve when a closure is
applied rather than when it is written:

```
loop = x => loop(x)
```

That terminates only by hitting the evaluation limits (§35), which is the
correct outcome for a Turing-complete language.

Definitions are runtime bindings. v0 does not persist them: source-code and
function persistence are non-goals (§2), and closures are not persistable
values (§13).

A definition shadows the atom of the same name, so defining `Alice` changes what
`knows(Alice)` means. Definitions also shadow primitives, so defining `holds`
replaces it.

## Functions build graph values

The interesting case is a function whose result is symbolic:

```
friend = x => y => knows(x)(y)

friend                 → x => y => knows(x)(y)
friend(Alice)          → y => knows(Alice)(y)
friend(Alice)(Bob)     → knows(Alice)(Bob)
```

Both lambdas reduce, then reduction stops because `knows` is inert. A function
has constructed a persistable fact, using nothing but ordinary application.

## Commands

Mutation is deliberately not an expression (§28, §33). Writes are commands.

```
command := "assert" term
         | "retract" claim
         | "match" pattern
         | "transaction" "{" { assert | retract } "}"
```

```
assert knows(Alice)(Bob)
retract claim:1
match knows(Alice)(?x)

transaction {
    assert knows(Alice)(Bob)
    assert knows(Bob)(Charlie)
}
```

`assert` evaluates its term first and claims the resulting value, so the world
holds evaluated values and never unevaluated programs (§29). `retract` targets
one exact claim, not every assertion of a value (§30). A transaction publishes
all of its commands or none of them, and a later command inside one observes an
earlier one (§31, §32).

Reading the world is *not* a command. `holds` is an ordinary primitive, so it
composes like anything else:

```
answer = holds(knows(Alice)(Bob))
assert observed(holds(knows(Alice)(Bob)))
```

`assert`, `retract`, `match` and `transaction` are contextual keywords. They
introduce a command only at the start of a statement, so `knows(assert)` is
still a perfectly good value.

## Patterns

Patterns belong to the query interface, not the calculus (§22):

```
knows(Alice)(?x)      ?relation(Alice)(Bob)      pair(?x)(?x)      _
```

`?x` is a capture, not a lambda variable. The two never mix. Repeating a capture
requires both positions to match the same value.

## Programs

A program is a sequence of statements, each a definition, a command, or a bare
expression. Statements need no separator. A term ends as soon as the next token
cannot continue it, and an application's `(` only continues a term on the same
line, so a line beginning with `(` starts a new statement.

`#` begins a comment that runs to the end of the line.

```
# A tiny complete program.
friend = x => y => knows(x)(y)

transaction {
    assert friend(Alice)(Bob)
    assert friend(Bob)(Charlie)
}

answer = holds(friend(Alice)(Bob))
match knows(?who)(?whom)
```

## The REPL

```
$ loom
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

## What is deliberately absent

No operators, `if`, blocks, loops, classes, methods, or `let`. No arithmetic:
write `add(2)(3)`, not `2 + 3`.

These are all future *surface* growth, and none of them needs the semantic
kernel to grow. `2 + 3` would lower to `add(2)(3)`, `a == b` to `equal(a)(b)`,
`!x` to `not(x)`, and

```
f = x => {
    y = expensive(x)
    add(y)(y)
}
```

would lower to

```
f = x => (y => add(y)(y))(expensive(x))
```

The discipline is that surface syntax may grow without the semantic kernel
growing.

Look is the other notation that will sit beside this one. It describes the same
values for a different reason:

| Loom | Look |
| --- | --- |
| `knows(Alice)(Bob)` | `[Alice] >[knows] [Bob]` |
| `holds(knows(Alice)(Bob))` | `[Alice] >[knows] [Bob] ?` |
| `match(knows(Alice)(?x))` | `[Alice] >[knows]` |

The arrow is useful notation, not a primitive edge (§43).
