# Loom canonical form v0.1

Status: normative. This is the byte encoding §36 of
[`loom-v0-spec.md`](loom-v0-spec.md) leaves to be specified separately.

§36 requires that two structurally equal persistable values serialize
identically. This document fixes the bytes, so that agreement holds across
implementations and not merely within one.

## Values

Only persistable values have a canonical form: atoms, and applications built
recursively from persistable values (§13). Closures do not, and neither does an
application over one.

```
value := atom | application
atom        := "Atom(" tag "," quoted ")"
application := "App(" value "," value ")"
tag         := "name" | "int" | "text"
```

No whitespace anywhere. The output is UTF-8.

```
Atom(name,"Alice")
App(App(Atom(name,"knows"),Atom(name,"Alice")),Atom(name,"Bob"))
```

Application associates left, so `gave(Alice)(Book)(Bob)` nests on the function
side:

```
App(App(App(Atom(name,"gave"),Atom(name,"Alice")),Atom(name,"Book")),Atom(name,"Bob"))
```

## Tags

The tag exists only so that atoms which would collide once written down stay
distinct: the name `42`, the integer `42` and the text `"42"` are three atoms.
It does not create different kinds of semantic node (§5).

| Tag | Body |
| --- | --- |
| `name` | the name as written |
| `int` | the canonical decimal form: no leading zeros, `-` only for negatives, `0` for zero |
| `text` | the text as written |

`042` and `42` are one atom, `Atom(int,"42")`. Leading zeros are notation, not
identity. Surface syntax has no leading `+`.

## Quoting

A body is written between `"` characters. Only what must be escaped is escaped:

| Byte | Written as |
| --- | --- |
| `"` | `\"` |
| `\` | `\\` |
| 0x0A | `\n` |
| 0x0D | `\r` |
| 0x09 | `\t` |
| other bytes < 0x20, and 0x7F | `\u00` followed by two **lowercase** hex digits |
| anything else | the literal byte |

Bytes above 0x7F pass through untouched. A payload that is not valid UTF-8
therefore survives a round trip byte for byte, and no implementation has to
agree with another about Unicode normalization or printability. This is the one
place where being less clever than a language's built-in string quoting matters:
Go's `strconv.Quote`, Python's `repr`, and JSON's encoder all disagree about
non-ASCII and control characters, so none of them can be the definition.

A reader must also accept `\uXXXX` for any code point and decode it to UTF-8,
even though a writer only ever emits that form for control bytes.

## Ordering

A set of values has no canonical order of its own. Where a listing must be
reproducible, sort the canonical forms as byte strings. The conformance corpus
does this for the values held at the end of a program.

Match results are a set and ordering carries no meaning (§25). Where they must
be serialized, sort the rendered bindings as byte strings.

## Surface form

`Source` writes a value in the syntax `Parse` accepts, which is not the
canonical form and is not used for equality:

```
knows(Alice)(Bob)
says(Alice)("hello")
```

Text literals in surface syntax use the same quoting table as above, so the two
forms agree about escaping even though they agree about nothing else.

`Source` and `Parse` are inverses over persistable values: parsing the surface
form of a value and evaluating it yields the same value. The corpus checks this
for every value a case leaves held.

## What is not canonicalized

Claim identities, world sequence numbers, and the order values were asserted in
are all local to one store and carry no cross-host meaning. Two implementations
that agree on every canonical value may number their claims differently.
