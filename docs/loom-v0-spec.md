# Loom Semantic Kernel v0

Status: proposed normative specification for the first clean implementation
Scope: Loom semantics only. Look, query planning, standard-library breadth, UI
concerns, and application-specific behavior are out of scope except where needed
to define the boundary.

This document intentionally starts smaller than the broader Loom/Look
architecture.

The goal of v0 is to establish a semantic foundation that can later support:

* a Turing-complete programming language;
* persistent graph-shaped data;
* structural querying;
* higher-order functions;
* reactive evaluation;
* Look as a graph-oriented query/update language.

The implementation should optimize aggressively, but it must not introduce
additional semantic concepts merely for convenience.

---

## 1. The idea

Loom has one universal composition operation:

```
F(X)
```

Application does not necessarily mean "call a function."

It means:

apply one value to another value.

Some applications reduce:

```
identity(A)
→ A
```

Some applications remain symbolic:

```
knows(Alice)
→ knows(Alice)
knows(Alice)(Bob)
→ knows(Alice)(Bob)
```

This gives Loom its central rule:

Everything evaluates to a value. Application is universal. Some applications
reduce; irreducible applications remain symbolic values.

Persistence operates on those resulting values.

Therefore:

```
knows(Alice)(Bob)
```

constructs a symbolic value.

```
holds(knows(Alice)(Bob))
```

asks whether that value is currently asserted.

And:

```
assert knows(Alice)(Bob)
```

asserts that value into the persistent world.

There is no primitive edge or relation type.

---

## 2. Non-goals of v0

The first implementation must not attempt to implement all of Loom or Look.

Specifically, v0 does not require:

```
arithmetic syntax
if syntax
lists
sets
map/filter/fold
transitive traversal
reactivity
valid_until
temporal intervals
Look arrows
row queries
query planning
schemas
facets
properties
named edges
multi-argument functions
operator syntax
source-code persistence
function persistence
distributed execution
authorization
```

Those belong above this semantic layer.

The purpose of v0 is to make the foundation unambiguous.

---

## 3. Semantic layers

Loom must distinguish three layers.

```
┌─────────────────────────────┐
│       Pure term layer       │
│                             │
│ atom · lambda · application │
└──────────────┬──────────────┘
               │ evaluated within
               ▼
┌─────────────────────────────┐
│        World snapshot       │
│                             │
│ holds · match               │
└──────────────┬──────────────┘
               │ changed only through
               ▼
┌─────────────────────────────┐
│       Transaction layer     │
│                             │
│ assert · retract            │
└─────────────────────────────┘
```

Pure computation must not perform mutation.

Reads may depend on a world snapshot.

Writes happen only through explicit transaction commands.

---

## 4. Terms

The semantic language has three meaningful constructors:

```
atom
abstraction
application
```

A textual representation additionally needs bound-variable references.

A minimal grammar is:

```
term :=
    atom
    | variable
    | variable "=>" term
    | term "(" term ")"
```

Examples:

```
Alice
x => x
identity(Alice)
f(x)(y)
```

Application associates left:

```
f(x)(y)
```

means:

```
(f(x))(y)
```

There is no primitive multi-argument call.

This:

```
f(x, y)
```

may later exist as surface sugar, but its meaning is:

```
f(x)(y)
```

---

## 5. Atoms

An atom is an irreducible named or literal value.

Examples:

```
Alice
Bob
knows
task
42
"hello"
true
false
```

The semantic kernel does not require separate categories for:

```
entity
relation
integer
string
boolean
function name
```

Those distinctions may matter to parsers, serializers, optimized
representations, or libraries.

They do not create different kinds of semantic node.

Atom identity must be stable and exact.

Two atoms are equal if and only if their canonical identities are equal.

---

## 6. Abstraction

An abstraction creates a function value.

```
x => body
```

Examples:

```
identity = x => x
constant = x => y => x
```

Evaluating an abstraction creates a closure over its lexical environment.

Conceptually:

```
Closure {
    parameter
    body
    environment
}
```

A closure is a runtime value.

The closure representation is not part of the persistent graph in v0.

---

## 7. Application

Application is the universal structural operation.

```
F(X)
```

First evaluate F.

Then evaluate X.

Then apply the resulting values.

v0 uses call-by-value evaluation.

Conceptually:

```
evaluate(F(X), env, world):
    function = evaluate(F, env, world)
    argument = evaluate(X, env, world)
    return apply(function, argument, world)
```

---

## 8. Applying a closure

If the left value is a closure:

```
(x => body)(value)
```

evaluate body in the closure's captured environment extended with:

```
x = value
```

Example:

```
(x => x)(Alice)
→ Alice
```

And:

```
(x => y => x)(Alice)(Bob)
→ Alice
```

Implementations should normally use environments, closures, de Bruijn indices,
or another capture-safe representation.

Literal textual substitution is not required.

---

## 9. Neutral application

If the left value has no reduction behavior, application does not fail.

Instead it produces a symbolic application value.

Example:

```
knows(Alice)
```

evaluates to the value:

```
App(knows, Alice)
```

Applying that again:

```
knows(Alice)(Bob)
```

produces:

```
App(
    App(knows, Alice),
    Bob
)
```

This resulting value is called a neutral application.

Neutral application is the bridge between computation and graph structure.

---

## 10. Normal examples

A lambda reduces:

```
(x => x)(Alice)
→ Alice
```

An inert atom does not:

```
knows(Alice)
→ knows(Alice)
```

Nested neutral application remains symbolic:

```
knows(Alice)(Bob)
→ knows(Alice)(Bob)
```

Application arguments are evaluated first.

Therefore:

```
knows((x => x)(Alice))(Bob)
```

reduces to:

```
knows(Alice)(Bob)
```

before becoming symbolic.

---

## 11. Primitive reducers

The minimal calculus does not require arithmetic or other host operations.

An implementation may nevertheless register efficient primitive reducers.

For example:

```
add
```

may have runtime reduction behavior such that:

```
add(2)(3)
→ 5
```

Primitive behavior is conceptually equivalent to library behavior, not a new
application syntax.

A primitive must therefore still participate through ordinary application.

The implementation must not introduce semantic forms such as:

```
BinaryAdd(2, 3)
```

when the Loom meaning is:

```
add(2)(3)
```

Primitives are optimizations and host boundaries, not an alternate calculus.

---

## 12. Values

Runtime values in v0 are:

```
Atom
Closure
NeutralApplication
```

Implementations may have additional internal representations such as:

```
PrimitivePartial
Thunk
InternedApplication
NativeInteger
```

provided they are observationally equivalent to the specified values.

---

## 13. Persistable values

Not every runtime value is persistable in v0.

A persistable value is recursively:

```
Atom
or
Application(
    persistable value,
    persistable value
)
```

Closures are not persistable in v0.

Therefore:

```
knows(Alice)(Bob)
```

is persistable.

But:

```
x => x
```

is not.

Attempting to persist a closure must produce:

```
non_persistable_value
```

This avoids prematurely defining closure equality, captured-environment
persistence, source identity, alpha-equivalence, and code versioning.

Code persistence can later be implemented through explicit reification.

---

## 14. Code is not silently data

Loom does not claim that executed code and data are literally
indistinguishable.

For example:

```
add(2)(3)
```

may evaluate to:

```
5
```

The original expression and its result are not the same value.

If Loom later needs to inspect or persist program structure, it should expose an
explicit operation such as:

```
quote(...)
```

or:

```
reify(...)
```

The safe principle is:

Loom code has a canonical structural representation that can be reified as data.

Not:

code and data are always literally identical.

---

## 15. Structural equality

Persistable values use structural equality.

Atoms:

```
A == B
```

iff their canonical atom identities are equal.

Applications:

```
F(X) == G(Y)
```

iff:

```
F == G
and
X == Y
```

Therefore:

```
knows(Alice)(Bob)
```

equals another independently constructed:

```
knows(Alice)(Bob)
```

No application-instance identity participates in semantic equality.

Implementations are strongly encouraged to hash-cons or intern persistable
application values so structural equality often becomes identity equality
internally.

That is an optimization, not semantics.

---

## 16. Closure equality

v0 defines no structural equality for closures.

Closures:

* cannot be persisted;
* cannot be structurally matched in the persistent world;
* need not have stable serialized identities.

An implementation may use runtime identity internally.

That identity must not leak into persistent Loom semantics.

---

## 17. The world

Persistent reads occur against an immutable world snapshot.

Conceptually:

```
World {
    sequence
    live_claims
}
```

Evaluation is therefore formally relative to a world:

```
World ⊢ expression ⇓ value
```

Pure expressions may ignore the world.

For example:

```
W₁ ⊢ (x => x)(Alice) ⇓ Alice
W₂ ⊢ (x => x)(Alice) ⇓ Alice
```

Persistent reads may differ:

```
W₁ ⊢ holds(knows(Alice)(Bob)) ⇓ true
W₂ ⊢ holds(knows(Alice)(Bob)) ⇓ false
```

Every single evaluation must observe one logically consistent snapshot.

---

## 18. Facts and claims

Loom distinguishes:

```
fact value
```

from:

```
claim asserting that value
```

The fact:

```
knows(Alice)(Bob)
```

is a persistable Loom value.

A claim is persistence metadata saying that some source asserted that value.

Conceptually:

```
Claim {
    id
    value
    asserted_sequence
    retracted_sequence?
    metadata
}
```

Multiple claims may assert the same value.

For example:

```
claim-1 → knows(Alice)(Bob)
claim-2 → knows(Alice)(Bob)
```

The fact remains held while at least one relevant live claim remains.

This distinction is required for provenance, independent writers, history, and
exact retraction.

---

## 19. holds

`holds` asks whether a persistable value is asserted in the current world.

Conceptually:

```
holds(X)
```

first evaluates X.

If the result is not persistable:

```
error non_persistable_value
```

Otherwise:

```
holds_W(X) = true
```

iff at least one live claim in W asserts a structurally equal value.

Examples:

```
holds(knows(Alice)(Bob))
```

may return:

```
true
```

while:

```
knows(Alice)(Bob)
```

by itself merely evaluates to the symbolic value:

```
knows(Alice)(Bob)
```

Construction and truth are separate operations.

---

## 20. Assertions may contain any persistable value

The kernel does not require a proposition type.

Therefore, in principle:

```
assert 42
holds(42)
```

is valid.

Likewise:

```
assert Alice
assert knows(Alice)(Bob)
assert color(Task)(red)
```

Higher layers may establish conventions about which values are useful to assert.

The kernel does not enforce them.

---

## 21. match

`match` structurally queries asserted persistable values.

It does not run arbitrary Loom functions over every value.

It consumes a pattern.

Patterns belong to the persistence/query interface, not the lambda calculus.

---

## 22. Patterns

The v0 pattern language contains:

```
constant(value)
capture(name)
wildcard
application(pattern, pattern)
```

Informal syntax may be written:

```
knows(Alice)(?x)
knows(?x)(Bob)
?relation(Alice)(Bob)
_
```

But `?x` is not a Loom lambda variable.

It is a pattern capture.

These are distinct concepts.

---

## 23. Pattern matching

Pattern matching is structural.

Pattern:

```
knows(Alice)(?x)
```

matches:

```
knows(Alice)(Bob)
```

with:

```
x = Bob
```

It does not match:

```
knows(Charlie)(Bob)
```

Pattern:

```
?relation(Alice)(Bob)
```

matches:

```
knows(Alice)(Bob)
parent(Alice)(Bob)
likes(Alice)(Bob)
```

and binds relation appropriately.

Repeated captures must agree structurally.

For example:

```
pair(?x)(?x)
```

matches:

```
pair(Alice)(Alice)
```

but not:

```
pair(Alice)(Bob)
```

---

## 24. What match searches

`match_W(pattern)` searches distinct persistable values currently held in W.

If three live claims assert the same value:

```
knows(Alice)(Bob)
```

a normal match sees that fact value once.

Claim-level querying is a separate persistence API.

Thus ordinary Loom graph queries are about facts.

Provenance queries are about claims.

---

## 25. match results

The semantic result of match is a set of binding environments.

For:

```
match(knows(Alice)(?x))
```

the result may conceptually be:

```
{
    {x: Bob},
    {x: Charlie},
    {x: Dana}
}
```

Ordering is not semantically significant in v0.

APIs requiring deterministic serialization should use a canonical deterministic
ordering.

This ordering must not change set semantics.

---

## 26. kind is not a property primitive

Suppose the world holds:

```
kind(Task)(task)
```

Then:

```
kind(Task)
```

does not evaluate to:

```
task
```

It evaluates to the neutral application:

```
kind(Task)
```

Therefore a future Look expression:

```
kind == task
```

should lower semantically to something such as:

```
holds(kind(self)(task))
```

or to a structural match.

If an application needs "the one object related by kind," that must be a derived
operation with explicit cardinality behavior.

For example:

```
one(match(kind(Task)(?x)))
```

Loom does not silently create property-accessor semantics.

---

## 27. Queryability

Arbitrary Loom programs are Turing complete.

Arbitrary Loom programs are therefore not necessarily efficiently queryable.

v0 makes no claim that:

```
x => arbitrary_program(x)
```

can be transformed into an index query.

The future architecture should distinguish:

```
Loom semantics
```

from:

```
the efficiently planable subset used by Look
```

Every Look query should have Loom meaning.

Not every Loom program needs to be a Look query.

This is an optimization boundary, not a second universe of truth.

---

## 28. Commands are not expressions

Mutation is deliberately excluded from ordinary term evaluation.

There is no pure expression rule:

```
evaluate(assert(X))
```

that secretly changes the database.

Instead Loom has a command/transaction layer.

v0 commands are:

```
assert term
retract claim-id
```

A transaction is an ordered sequence of commands.

---

## 29. Assertion command

Executing:

```
assert X
```

inside a transaction performs:

```
value = evaluate(X, transaction_world)
```

The result must be persistable.

Then a new claim is created:

```
Claim {
    id = fresh
    value = value
    ...
}
```

The command returns the claim id to the transaction/runtime.

Therefore:

```
assert (x => x)(Alice)
```

asserts:

```
Alice
```

And:

```
assert knows(Alice)(Bob)
```

asserts:

```
knows(Alice)(Bob)
```

The persistent world contains evaluated values, not unevaluated programs.

---

## 30. Retraction command

The primitive retraction operation targets a claim identity:

```
retract claim-123
```

It does not mean:

```
delete every assertion of some equal fact value
```

This avoids ambiguity when multiple independent claims support the same fact.

Higher-level APIs may provide convenience operations such as:

```
retractOwnClaims(value)
```

or:

```
retractAll(value)
```

but these must have explicit policy.

---

## 31. Transaction semantics

A transaction:

```
transaction {
    command₁
    command₂
    ...
}
```

has these guarantees:

1. Commands execute in source order.
2. Each command executes exactly once.
3. Commands may observe earlier staged changes in the same transaction.
4. No outside observer sees a partial transaction.
5. Successful commit creates one new world sequence.
6. Failed transactions publish no changes.

This keeps mutation semantics independent of Loom evaluation strategy.

---

## 32. Snapshot semantics inside transactions

A transaction starts from world:

```
W₀
```

and constructs staged worlds:

```
W₀
→ W₁
→ W₂
→ ...
→ Wₙ
```

Each command evaluates against the current staged world.

For example:

```
transaction {
    c = assert knows(Alice)(Bob)
    assert observed(holds(knows(Alice)(Bob)))
}
```

the second command sees the first staged assertion.

Outside readers continue observing the pre-transaction snapshot until commit.

---

## 33. No lazy effects

Because writes are commands rather than ordinary expressions:

* they cannot disappear because a value is unused;
* they cannot execute twice because a thunk is evaluated twice;
* they cannot reorder because an optimizer changes pure evaluation;
* they cannot occur accidentally during graph inspection.

This separation is normative.

---

## 34. Errors

v0 should define at least these semantic errors:

```
unbound_variable
non_persistable_value
unknown_claim
already_retracted_claim
invalid_pattern
resource_limit
primitive_error
```

Syntax errors belong to the parser rather than the semantic kernel.

An ordinary neutral application is not an error.

For example:

```
42(Alice)
```

may simply evaluate to the symbolic value:

```
42(Alice)
```

unless a higher layer chooses to reject such structures.

The kernel remains permissive.

---

## 35. Resource policy

Turing completeness implies that evaluation may not terminate.

v0 therefore requires implementation-level resource controls.

At minimum:

```
maximum evaluation steps
maximum call depth or equivalent stack protection
maximum term/application size
maximum match result count
```

Reaching a configured limit produces:

```
resource_limit
```

The limits themselves are deployment policy and need not be language constants.

---

## 36. Canonical structural representation

Persistable values must have a canonical structural representation.

Conceptually:

```
Atom("Alice")
App(
    App(
        Atom("knows"),
        Atom("Alice")
    ),
    Atom("Bob")
)
```

Two structurally equal persistable values must serialize identically.

Canonicalization is required for:

```
structural equality
hashing
deduplication
indexes
content addressing
cross-host conformance
```

The exact byte encoding should be specified separately.

---

## 37. Recommended internal representation

Not normative, but the first implementation should strongly consider:

```
ValueId
Atom {
    canonical_payload
}
App {
    function: ValueId
    argument: ValueId
}
```

with hash-consing:

```
(function, argument) -> canonical ValueId
```

Then:

```
knows(Alice)(Bob)
```

has one canonical application identity regardless of how many times it is
constructed.

This makes:

```
equality
holds
claim storage
matching
memoization
```

much cheaper while preserving the minimal semantics.

Closures should remain runtime objects outside this canonical persistable-value
arena.

---

## 38. Recommended persistence model

Again non-normative, but a clean v0 implementation could use an append-only
claim journal:

```
ASSERT claim-id value-id metadata
RETRACT claim-id
```

A world snapshot is a sequence prefix.

Indexes may maintain:

```
value-id -> live claim ids
application function-id -> application ids
application argument-id -> application ids
application pair(function-id, argument-id) -> application id
```

Later, specialized indexes may recognize application spines such as:

```
knows(Alice)(Bob)
```

without making `(subject, relation, object)` a semantic primitive.

---

## 39. Application spine

A nested application can be viewed as a head and ordered arguments.

For:

```
gave(Alice)(Book)(Bob)
```

the application spine is:

```
head = gave
args = [
    Alice,
    Book,
    Bob
]
```

This is a derived structural view.

It is useful for:

```
indexing
matching
debugging
Look lowering
query optimization
```

But the canonical semantics remain binary application.

The spine must not become a second representation with different meaning.

---

## 40. Initial conformance cases

An implementation is not v0-conformant until these behaviors are pinned by
tests.

### 40.1 Atom

```
Alice
→ Alice
```

### 40.2 Identity

```
(x => x)(Alice)
→ Alice
```

### 40.3 Constant

```
(x => y => x)(Alice)(Bob)
→ Alice
```

### 40.4 Neutral application

```
knows(Alice)
→ knows(Alice)
```

### 40.5 Nested neutral application

```
knows(Alice)(Bob)
→ knows(Alice)(Bob)
```

### 40.6 Evaluate arguments before neutralization

```
knows((x => x)(Alice))(Bob)
→ knows(Alice)(Bob)
```

### 40.7 Structural equality

Two independently constructed:

```
knows(Alice)(Bob)
```

must be equal.

### 40.8 Assert symbolic value

```
assert knows(Alice)(Bob)
```

creates a claim whose value is structurally:

```
knows(Alice)(Bob)
```

### 40.9 Holds

After that assertion:

```
holds(knows(Alice)(Bob))
→ true
```

### 40.10 Missing fact

```
holds(knows(Alice)(Charlie))
→ false
```

### 40.11 Match

Given:

```
knows(Alice)(Bob)
knows(Alice)(Charlie)
knows(Dana)(Bob)
```

then:

```
match(knows(Alice)(?x))
```

returns bindings equivalent to:

```
{x = Bob}
{x = Charlie}
```

### 40.12 Duplicate claims

After two claims assert:

```
knows(Alice)(Bob)
```

retracting one claim leaves:

```
holds(knows(Alice)(Bob))
→ true
```

Retracting both makes it false.

### 40.13 Closure persistence rejection

```
assert (x => x)
```

must fail with:

```
non_persistable_value
```

### 40.14 Snapshot consistency

An evaluation against world W₁ must not observe a transaction committed after W₁
was acquired.

### 40.15 Transaction visibility

Commands later in one transaction must observe earlier staged commands from that
transaction.

---

## 41. Minimal implementation milestone

The first engineering milestone should implement only:

```
atoms
bound variables
closures
unary application
neutral applications
structural equality
canonical application interning
immutable world snapshots
claim assertion
claim retraction
holds
structural match
transactions
conformance tests
```

Do not implement Look yet.

Do not implement arithmetic unless useful for smoke tests.

Do not implement graph traversal syntax.

Do not implement reactive invalidation.

Do not implement temporal validity.

Do not implement a large standard library.

If this milestone is clean, those become layering exercises rather than semantic
redesigns.

---

## 42. Second milestone

Only after v0 semantics are stable should the next layer introduce a small Loom
Core.

Candidate first functions:

```
true
false
if
pair
first
second
equal
not
and
or
list
map
filter
fold
one
count
claims
match
holds
```

Primitive machine integers and strings may be added for efficiency.

Their behavior should still fit ordinary application.

---

## 43. Look comes after Loom Core

Look should not define new truth semantics.

For example, future Look:

```
[Alice] >[knows] [Bob] ?
```

can lower to:

```
holds(knows(Alice)(Bob))
```

And:

```
[Alice] >[knows] ?
```

can lower to a pattern match equivalent to:

```
match(knows(Alice)(?x))
```

The arrow is useful notation.

It is not a primitive edge.

Likewise:

```
(? kind == task)
```

should lower to semantics equivalent to:

```
match(kind(?x)(task))
```

or:

```
holds(kind(x)(task))
```

depending on context.

Look may provide a much richer query grammar, provided its meaning is reducible
to Loom semantics.

---

## 44. Design law

When adding anything to Loom, ask:

1. Can this be expressed with ordinary application?
2. Can this be implemented as a Loom/Core function?
3. Is it only convenient syntax?
4. Is it merely an index or optimizer?
5. Does it require observing the persistent world?
6. Does it require changing the persistent world?

Only genuinely irreducible concepts belong below the corresponding boundary.

The implementation should prefer:

minimal semantics, rich derivation, aggressive optimization.

---

## 45. Normative summary

The Loom v0 semantic model is:

1. Atoms are values.
2. Lambdas evaluate to function closures.
3. `F(X)` is the universal composition form.
4. Applying a closure reduces by lexical binding.
5. Applying an inert value creates a neutral symbolic application.
6. Therefore some applications compute while others remain graph structure.
7. Persistable values are atoms and neutral applications built recursively from
   them.
8. Closures are runtime values but are not persistable in v0.
9. The persistent world contains claims asserting persistable values.
10. Multiple claims may assert the same value.
11. `holds(X)` evaluates X and asks whether at least one live claim asserts that
    value.
12. `match(pattern)` structurally matches currently held values.
13. Pattern captures are not lambda variables.
14. Reads occur against one immutable world snapshot.
15. Writes are transaction commands, not lazy expressions.
16. `assert` evaluates its term before creating a claim.
17. `retract` targets an exact claim identity.
18. Persistent-value equality is structural.
19. Relations and edges are not primitive Loom concepts.
20. Look is a later graph-oriented notation over these semantics.

The foundational idea can therefore be stated in four lines:

```
Everything evaluates to a value.
Application is universal.
Some applications reduce.
The rest are symbolic graph structure.
```

Persistence adds only:

```
asserted values
claims
holds
match
```

That is the first Loom that should be implemented.
