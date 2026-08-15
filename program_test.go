package loom_test

import (
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/felixubl/loom"
)

// run feeds statements to a session and returns what each one produced, in the
// shape the REPL prints.
func run(t *testing.T, s *loom.Session, src string) []string {
	t.Helper()
	program, err := s.Store().ParseProgram(src)
	if err != nil {
		t.Fatalf("parse program: %v", err)
	}
	results, err := s.RunProgram(program)
	if err != nil {
		t.Fatalf("run program: %v", err)
	}
	var out []string
	for _, r := range results {
		switch r.Command.(type) {
		case loom.Evaluate:
			out = append(out, s.Store().Display(r.Value))
		case loom.Assert, loom.Transaction:
			for _, id := range r.Claims {
				out = append(out, "claim:"+itoa(uint64(id)))
			}
		case loom.Query:
			for _, row := range r.Rows {
				out = append(out, renderRow(s.Store(), row))
			}
		}
	}
	return out
}

func itoa(n uint64) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func renderRow(store *loom.Store, row loom.Bindings) string {
	names := make([]string, 0, len(row))
	for name := range row {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+" = "+store.Source(row[name]))
	}
	return strings.Join(parts, ", ")
}

func wantLines(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d lines %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

// The REPL transcript this milestone was specified by.
func TestReplTranscript(t *testing.T) {
	s := loom.NewSession(loom.New())
	got := run(t, s, `
		x => x
		(x => x)(Alice)
		knows(Alice)
		knows(Alice)(Bob)
		assert knows(Alice)(Bob)
		holds(knows(Alice)(Bob))
		match knows(Alice)(?x)
	`)
	wantLines(t, got,
		"x => x",
		"Alice",
		"knows(Alice)",
		"knows(Alice)(Bob)",
		"claim:1",
		"true",
		"x = Bob",
	)
}

// A function may construct a symbolic graph value: reduction runs out when it
// reaches an inert name, and what is left is the fact.
func TestFunctionsConstructGraphValues(t *testing.T) {
	s := loom.NewSession(loom.New())
	got := run(t, s, `
		identity = x => x
		friend   = x => y => knows(x)(y)

		transaction {
		    assert friend(Alice)(Bob)
		    assert friend(Bob)(Charlie)
		}

		friend(Alice)
		friend(Alice)(Bob)
		identity(Alice)
		holds(friend(Alice)(Bob))
		match knows(?who)(?whom)
	`)
	wantLines(t, got,
		"claim:1",
		"claim:2",
		"y => knows(x)(y)",
		"knows(Alice)(Bob)",
		"Alice",
		"true",
		"who = Alice, whom = Bob",
		"who = Bob, whom = Charlie",
	)
}

// A reference nothing defines is the atom of its name; defining that name
// changes what the same source means.
func TestDefinitionsShadowAtoms(t *testing.T) {
	s := loom.NewSession(loom.New())
	wantLines(t, run(t, s, "likes(Alice)"), "likes(Alice)")
	wantLines(t, run(t, s, `
		Alice = knows(Bob)
		likes(Alice)
	`), "likes(knows(Bob))")
}

// A lambda parameter shadows a top-level definition, because the parser
// resolves bound names before anything else sees them.
func TestParameterShadowsDefinition(t *testing.T) {
	s := loom.NewSession(loom.New())
	wantLines(t, run(t, s, `
		identity = x => x
		shadowed = identity => identity(Alice)
		identity(Alice)
		shadowed(g)
	`), "Alice", "g(Alice)")
}

// References resolve when a closure is applied, not when it is written, so a
// definition can call itself. The depth limit is what stops it.
func TestRecursiveDefinition(t *testing.T) {
	s := loom.NewSession(loom.NewWithLimits(loom.Limits{Depth: 128}))
	program, err := s.Store().ParseProgram(`
		loop = x => loop(x)
		loop(Alice)
	`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunProgram(program); !errors.Is(err, loom.ErrResourceLimit) {
		t.Fatalf("got %v, want resource_limit", err)
	}
}

// Scoping is lexical: a closure carries the environment it was defined in, and
// applying it never resolves its free names against the caller's environment.
// Rebinding a name a function uses cannot change what that function means.
func TestClosuresAreLexicallyScoped(t *testing.T) {
	s := loom.NewSession(loom.New())
	wantLines(t, run(t, s, `
		helper = x => A
		f      = x => helper(x)
		g      = f
		g(Alice)
	`), "A")

	// A later binding of helper shadows the name for new code only.
	wantLines(t, run(t, s, `
		helper = x => B
		g(Alice)
		helper(Alice)
	`), "A", "B")
}

// The closure's environment travels with it, so it keeps working when it is
// applied through the bare kernel API with no session in sight.
func TestClosureCarriesItsEnvironmentThroughTheKernelAPI(t *testing.T) {
	s := loom.NewSession(loom.New())
	run(t, s, `
		helper = x => greeted(x)
		f      = x => helper(x)
	`)
	fn, ok := s.Env().Lookup("f")
	if !ok {
		t.Fatal("f is not bound")
	}
	v, err := s.Store().World().Apply(fn, loom.Atom{ID: s.Store().Atom(loom.NameAtom("Alice"))})
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Store().Display(v); got != "greeted(Alice)" {
		t.Fatalf("got %q, want greeted(Alice)", got)
	}
}

// Collecting every top-level name before resolving any body is what lets a
// definition refer to one written further down, and two refer to each other.
func TestForwardAndMutualReferences(t *testing.T) {
	s := loom.NewSession(loom.New())
	wantLines(t, run(t, s, `
		a = b
		b = Alice
		a
	`), "Alice")

	s = loom.NewSession(loom.New())
	wantLines(t, run(t, s, `
		even = n => odd(n)
		odd  = n => reached(n)
		even(Alice)
	`), "reached(Alice)")
}

// A definition that depends on its own value, rather than merely on its own
// name, is an error rather than a hang.
func TestCyclicDefinition(t *testing.T) {
	s := loom.NewSession(loom.New())
	program, err := s.Store().ParseProgram("a = b\nb = a\na")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunProgram(program); !errors.Is(err, loom.ErrCyclicDefinition) {
		t.Fatalf("got %v, want cyclic_definition", err)
	}
}

func TestDuplicateDefinition(t *testing.T) {
	s := loom.NewSession(loom.New())
	program, err := s.Store().ParseProgram("a = Alice\na = Bob")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunProgram(program); !errors.Is(err, loom.ErrDuplicateDefinition) {
		t.Fatalf("got %v, want duplicate_definition", err)
	}

	// Across statements it is ordinary shadowing, which a REPL depends on.
	s = loom.NewSession(loom.New())
	wantLines(t, run(t, s, "a = Alice\n"))
	wantLines(t, run(t, s, "a = Bob\n"))
	wantLines(t, run(t, s, "a"), "Bob")
}

// Reads and writes are different sides of the language, and match is a read.
func TestMutatesBoundary(t *testing.T) {
	s := loom.New()
	pat, err := s.ParsePattern("knows(?x)")
	if err != nil {
		t.Fatal(err)
	}
	reads := []loom.Command{
		loom.Evaluate{Term: loom.TermAtom{Payload: loom.NameAtom("Alice")}},
		loom.Query{Pattern: pat},
		loom.Define{Name: "a", Term: loom.TermAtom{Payload: loom.NameAtom("Alice")}},
	}
	writes := []loom.Command{
		loom.Assert{Term: loom.TermAtom{Payload: loom.NameAtom("Alice")}},
		loom.Retract{Claim: 1},
		loom.Transaction{},
	}
	for _, c := range reads {
		if loom.Mutates(c) {
			t.Errorf("%T reported as a mutation", c)
		}
	}
	for _, c := range writes {
		if !loom.Mutates(c) {
			t.Errorf("%T reported as a read", c)
		}
	}
}

// §31.6: if any command in a transaction fails, none of them is published.
func TestTransactionIsAtomic(t *testing.T) {
	s := loom.NewSession(loom.New())
	program, err := s.Store().ParseProgram(`
		transaction {
		    assert knows(Alice)(Bob)
		    assert (x => x)
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunProgram(program); !errors.Is(err, loom.ErrNonPersistableValue) {
		t.Fatalf("got %v, want non_persistable_value", err)
	}
	wantLines(t, run(t, s, "holds(knows(Alice)(Bob))"), "false")
	if got := s.Store().World().Seq(); got != 0 {
		t.Fatalf("world sequence advanced to %d after a failed transaction", got)
	}
}

// §32: a later command in a transaction observes an earlier one.
func TestTransactionStagedVisibility(t *testing.T) {
	s := loom.NewSession(loom.New())
	run(t, s, `
		transaction {
		    assert knows(Alice)(Bob)
		    assert observed(holds(knows(Alice)(Bob)))
		}
	`)
	wantLines(t, run(t, s, "holds(observed(true))"), "true")
}

func TestRetractCommand(t *testing.T) {
	for _, form := range []string{"retract 1", "retract claim:1"} {
		s := loom.NewSession(loom.New())
		run(t, s, "assert knows(Alice)(Bob)")
		wantLines(t, run(t, s, "holds(knows(Alice)(Bob))"), "true")
		run(t, s, form)
		wantLines(t, run(t, s, "holds(knows(Alice)(Bob))"), "false")
	}
}

func TestTransactionRejectsOtherStatements(t *testing.T) {
	s := loom.New()
	for _, src := range []string{
		"transaction { x = Alice }",
		"transaction { knows(Alice) }",
		"transaction { match knows(?x) }",
		"transaction { assert knows(Alice)(Bob)",
	} {
		if _, err := s.ParseProgram(src); err == nil {
			t.Errorf("parsed %q, expected a syntax error", src)
		}
	}
}

// The statement keywords are contextual: they only introduce a command at the
// start of a statement.
func TestKeywordsAreContextual(t *testing.T) {
	s := loom.NewSession(loom.New())
	wantLines(t, run(t, s, `
		knows(assert)
		f = assert => assert(match)
		f(g)
	`), "knows(assert)", "g(match)")
}

// A newline ends a statement that is complete there. Where the grammar still
// requires a term it is ignored, so a body may sit on the line below.
func TestStatementsAreLineDelimited(t *testing.T) {
	s := loom.NewSession(loom.New())

	// A complete statement never absorbs the line below it. This is the case
	// the rule exists for.
	wantLines(t, run(t, s, "x => x\n(x => x)(Alice)"), "x => x", "Alice")
	wantLines(t, run(t, s, "knows\n(Alice)"), "knows", "Alice")

	// Where a term is still required, the newline carries no meaning.
	wantLines(t, run(t, s, "f = x =>\n  knows(x)\nf(Alice)"), "knows(Alice)")
	wantLines(t, run(t, s, "g =\n  Alice\ng"), "Alice")
	wantLines(t, run(t, s, "assert\n  knows(Alice)(Bob)"), "claim:1")

	// Parentheses suspend the rule outright.
	wantLines(t, run(t, s, "knows(\n  Alice\n)"), "knows(Alice)")
	wantLines(t, run(t, s, "h = (x =>\n  likes(x))\nh(Alice)"), "likes(Alice)")

	// Running out of input entirely is still an error, and is reported as
	// incomplete so a REPL knows to keep reading.
	for _, src := range []string{"f =", "f = x =>", "knows(", "transaction {"} {
		_, err := s.Store().ParseProgram(src)
		var syntax *loom.SyntaxError
		if !errors.As(err, &syntax) {
			t.Errorf("parsed %q, expected a syntax error", src)
		} else if !syntax.Incomplete {
			t.Errorf("%q reported as wrong rather than incomplete", src)
		}
	}
}

func TestComments(t *testing.T) {
	s := loom.NewSession(loom.New())
	wantLines(t, run(t, s, `
		# a leading comment
		identity = x => x   # trailing
		identity(Alice)     # and here
	`), "Alice")
}

func TestDisplayOfNonPersistableValues(t *testing.T) {
	s := loom.NewSession(loom.New())
	if err := s.Store().Register("add", loom.Primitive{Arity: 2, Apply: func(*loom.Context, []loom.Value) (loom.Value, error) {
		return nil, errors.New("unused")
	}}); err != nil {
		t.Fatal(err)
	}
	wantLines(t, run(t, s, `
		x => y => x
		add(2)
		wraps(x => x)
	`), "x => y => x", "add(2)", "wraps(x => x)")
}

func TestParseProgramErrors(t *testing.T) {
	s := loom.New()
	for _, src := range []string{"x =", "retract", "retract Alice", "retract 0", "match", "transaction", "= Alice"} {
		if _, err := s.ParseProgram(src); err == nil {
			t.Errorf("parsed %q, expected a syntax error", src)
		}
	}
}
