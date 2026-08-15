# Loom surface syntax and resolution v0

Status: normative for syntax, name resolution, and scoping.
[`loom-v0-spec.md`](loom-v0-spec.md) is normative for semantics and takes
precedence wherever the two meet.

Two different languages are involved and confusing them makes everything harder.
Go is the *host*: it implements the interpreter, and a user never needs to know
it exists. Loom is the language described here.

Nothing below adds meaning to the kernel. Every construct either denotes a term
the kernel already has, or is a command the kernel already defines (§28). The
governing discipline is that **surface syntax may grow without the semantic
kernel growing**.

## The pipeline

```
source
  ↓ parse
Name / Atom / Lambda / Apply
  ↓ collect top-level definitions
  ↓ resolve
Atom / Var / Def / Lambda / Apply
  ↓ evaluate
Atom / Closure / Intrinsic / NeutralApplication
```

Parsing decides nothing about what a name denotes, because that is not a
syntactic question. Resolution is a separate phase, and it is what makes scoping
lexical.

## Terms

```
term := atom | name | name "=>" term | term "(" term ")"
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

## Lines

Normative:

> A newline terminates a statement that is **complete** there. Where the grammar
> still requires a term, a newline carries no meaning. An unparenthesized
> application may only continue on the same physical line. Explicit delimiters
> may span lines.

The first sentence is the point, and it is not semicolon insertion. Insertion is
dangerous because it puts a terminator into complete-looking code by heuristic.
This rule only ever declines to terminate where terminating is impossible, and
there is no heuristic in that: `f = x =>` is not a statement under any reading.

A complete statement never absorbs the line below it:

```
knows                two statements: the second is (Alice)
(Alice)
```

A statement that cannot end yet simply continues:

```
friends = x =>       one statement
    knows(x)

answer =             one statement
    knows(Alice)(Bob)

assert               one statement
    knows(Alice)(Bob)
```

Parentheses suspend the rule outright, which is how a term spans lines when
nothing else would let it:

```
knows(
    Alice
)
```

Running out of input entirely is still an error. Parsers report it as
*incomplete* rather than wrong, which is how the REPL knows to keep reading
instead of complaining at a half-typed definition.

Only parentheses suspend line significance. A `transaction { … }` block still
takes one statement per line.

`#` begins a comment that runs to the end of the line.

## Name resolution

Normative precedence for a bare name:

1. a lexically bound lambda parameter
2. a top-level definition in scope
3. a base-environment binding, such as an intrinsic
4. otherwise, the atom of that name

Step 4 is what lets computation and graph construction share one notation. In
this program `identity` resolves to a function and `knows` falls through to an
atom, with nothing distinguishing them at the call site:

```
identity = x => x

identity(Alice)        → Alice
knows(Alice)           → knows(Alice)
knows(Alice)(Bob)      → knows(Alice)(Bob)
```

Reduction simply runs out when it reaches something with no reduction behavior,
and what is left is graph structure.

## Scoping is lexical

A closure captures the environment it was defined in, and that environment
travels with it. Applying a function never resolves its free names against the
caller's environment.

```
helper = x => A
f      = x => helper(x)
g      = f
```

Rebinding `helper` afterwards does not reach into `g`:

```
helper = x => B

g(Alice)          → A
helper(Alice)     → B
```

A function means what it meant where it was defined. This matters more in Loom
than in most languages, because long-lived computation is meant to sit beside
long-lived graph state.

Redefining a name shadows it rather than overwriting it, which is the same rule
seen from the other side.

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

Every top-level name in a program is collected before any body is resolved, so a
definition may refer to one written further down, and two may refer to each
other:

```
a = b
b = Alice          a is Alice

even = n => odd(n)
odd  = n => reached(n)
```

Recursion needs no special form and no dynamic lookup, because a definition's
cell exists before its term is evaluated:

```
loop = x => loop(x)
```

That terminates only by reaching the evaluation limits (§35), which is the
correct outcome for a Turing-complete language.

A definition that depends on its own *value* rather than merely on its own name
is an error, not a hang:

```
a = b
b = a              cyclic_definition
```

Defining the same name twice in one program is `duplicate_definition`. Across
separate statements it is ordinary shadowing.

Definitions are runtime bindings. v0 does not persist them: source-code and
function persistence are non-goals (§2), and closures are not persistable values
(§13).

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

## Intrinsics

Host reduction behavior is a first-class value, and its **identity is the value,
not the name it is bound under**. The base environment starts out binding
`holds` to the holds intrinsic.

Rebinding the name is legal and rebinds only the name:

```
holds = x => x
```

The intrinsic still exists and still means what it meant. Nothing that was
compiled against it changes.

This is the boundary between language names and semantic primitives. Anything
compiled should hold the intrinsic value rather than look up the text of a name.
When Look lowers

```
[Alice] >[knows] [Bob] ?
```

it should produce

```
Intrinsic.HOLDS(knows(Alice)(Bob))
```

and never `Ref("holds")(…)`, so that a program defining `holds = x => false`
cannot make Look queries stop working.

Namespaced access to shadowed intrinsics (`core.holds`) is a later concern. v0
does not add syntax for it.

## Reads and writes

The two sides of the language are deliberately separate.

| Side | Constructs | Rule |
| --- | --- | --- |
| Read | expressions, `holds`, `match` | evaluate against one world snapshot, change nothing |
| Write | `assert`, `retract`, `transaction` | the only things that change anything |

`match` has its own statement syntax, but it is a read: it evaluates against a
snapshot and is repeatable. It is not a mutation command. That boundary will
matter for caching and reactivity.

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

Reading the world is not a command, so `holds` composes like anything else:

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
expression.

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

The REPL keeps reading while the input is incomplete, so a definition may be
typed over several lines and a `transaction { … }` block over several more.

Each submitted statement is its own program, so a statement cannot see a name
introduced after it. Typing `f = x => helper(x)` before `helper` exists resolves
`helper` to an atom, and defining `helper` afterwards does not change `f`. That
is the same static resolution everything else benefits from, and it is not
something a different scoping rule could fix: resolution happens once, so a name
that was not there is not there. Put forward or mutually recursive definitions in
one file and load it with `loom -i`.

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

Look is the other notation that will sit beside this one. It describes the same
values for a different reason:

| Loom | Look |
| --- | --- |
| `knows(Alice)(Bob)` | `[Alice] >[knows] [Bob]` |
| `holds(knows(Alice)(Bob))` | `[Alice] >[knows] [Bob] ?` |
| `match(knows(Alice)(?x))` | `[Alice] >[knows]` |

The arrow is useful notation, not a primitive edge (§43).
