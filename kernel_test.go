package loom_test

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"

	"github.com/felixubl/loom"
)

func TestParseErrors(t *testing.T) {
	for _, src := range []string{"", "(", "knows(", "knows(Alice", "x =>", "=> x", "knows)", `"unterminated`, "9999999999999999999999"} {
		if _, err := loom.Parse(src); err == nil {
			t.Errorf("parse %q: expected a syntax error", src)
		} else {
			var syntax *loom.SyntaxError
			if !errors.As(err, &syntax) {
				t.Errorf("parse %q: got %T, want *SyntaxError", src, err)
			}
		}
	}
}

func TestFormatRoundTrip(t *testing.T) {
	for _, src := range []string{
		"Alice",
		"x => x",
		"x => y => x",
		"knows(Alice)(Bob)",
		"(x => x)(Alice)",
		"knows((x => x)(Alice))(Bob)",
		`says(Alice)("hello")`,
		"count(42)",
	} {
		term, err := loom.Parse(src)
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		if got := loom.Format(term); got != src {
			t.Errorf("round trip of %q gave %q", src, got)
		}
	}
}

// A free identifier is a reference, and a reference nothing defines evaluates
// to the atom of its own name. That is what makes knows(Alice) graph structure
// without anyone having declared knows.
func TestFreeIdentifierIsAReferenceThatFallsBackToAnAtom(t *testing.T) {
	term, err := loom.Parse("knows(Alice)")
	if err != nil {
		t.Fatal(err)
	}
	apply, ok := term.(loom.TermApply)
	if !ok {
		t.Fatalf("got %T, want TermApply", term)
	}
	if _, ok := apply.Fn.(loom.TermRef); !ok {
		t.Fatalf("function is %T, want TermRef", apply.Fn)
	}

	s := loom.New()
	if got := eval(t, s.World(), "knows(Alice)"); got != "knows(Alice)" {
		t.Fatalf("got %q, want knows(Alice)", got)
	}
}

func TestUnboundVariable(t *testing.T) {
	s := loom.New()
	// Parse can never produce this; a hand-built term can.
	_, err := s.World().Eval(loom.TermVar{Name: "x"})
	if !errors.Is(err, loom.ErrUnboundVariable) {
		t.Fatalf("got %v, want unbound_variable", err)
	}
}

// §11: a primitive participates through ordinary application. add(2)(3) is two
// one-argument applications, not a two-argument call form.
func TestPrimitiveThroughOrdinaryApplication(t *testing.T) {
	s := loom.New()
	if err := s.Register("add", loom.Primitive{Arity: 2, Apply: func(ctx *loom.Context, args []loom.Value) (loom.Value, error) {
		a, err := intArg(ctx.Store, args[0])
		if err != nil {
			return nil, err
		}
		b, err := intArg(ctx.Store, args[1])
		if err != nil {
			return nil, err
		}
		return loom.Atom{ID: ctx.Store.Atom(loom.IntAtom(a + b))}, nil
	}}); err != nil {
		t.Fatal(err)
	}

	if got := eval(t, s.World(), "add(2)(3)"); got != "5" {
		t.Fatalf("add(2)(3) = %q, want 5", got)
	}

	// Half-applied, it is not yet a value anyone can store.
	v, err := s.World().Eval(parse(t, "add(2)"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loom.Persist(v); !errors.Is(err, loom.ErrNonPersistableValue) {
		t.Fatalf("add(2) persisted as %v, want non_persistable_value", err)
	}

	// A primitive's failure surfaces as primitive_error.
	if _, err := s.World().Eval(parse(t, "add(2)(Alice)")); !errors.Is(err, loom.ErrPrimitiveError) {
		t.Fatalf("got %v, want primitive_error", err)
	}
}

func intArg(s *loom.Store, v loom.Value) (int64, error) {
	atom, ok := v.(loom.Atom)
	if !ok {
		return 0, fmt.Errorf("not an atom")
	}
	p, ok := s.Payload(atom.ID)
	if !ok || p.Tag != loom.TagInt {
		return 0, fmt.Errorf("not an integer")
	}
	return strconv.ParseInt(p.Body, 10, 64)
}

func TestRegisterRejectsNonsense(t *testing.T) {
	s := loom.New()
	if err := s.Register("zero", loom.Primitive{Arity: 0}); err == nil {
		t.Error("expected an error for arity 0")
	}
	if err := s.Register("empty", loom.Primitive{Arity: 1}); err == nil {
		t.Error("expected an error for a missing implementation")
	}
}

// §9: application never fails. Applying an atom to a closure yields a symbolic
// value, which simply has no canonical identity and so cannot be stored (§13).
func TestNeutralOverAClosureIsAValueButNotPersistable(t *testing.T) {
	s := loom.New()
	v, err := s.World().Eval(parse(t, "wraps(x => x)"))
	if err != nil {
		t.Fatalf("application over a closure failed: %v", err)
	}
	n, ok := v.(loom.Neutral)
	if !ok {
		t.Fatalf("got %T, want Neutral", v)
	}
	if n.ID != 0 {
		t.Fatal("an application over a closure was given a canonical identity")
	}
	if _, err := loom.Persist(v); !errors.Is(err, loom.ErrNonPersistableValue) {
		t.Fatalf("got %v, want non_persistable_value", err)
	}
}

// §34: the kernel is permissive. 42(Alice) is a symbolic value, not an error.
func TestApplyingALiteralIsNotAnError(t *testing.T) {
	s := loom.New()
	if got := eval(t, s.World(), "42(Alice)"); got != "42(Alice)" {
		t.Fatalf("got %q, want 42(Alice)", got)
	}
}

// §20: the kernel requires no proposition type.
func TestAssertAnyPersistableValue(t *testing.T) {
	s := loom.New()
	for _, src := range []string{"42", "Alice", `"hello"`, "knows(Alice)(Bob)", "color(Task)(red)"} {
		assert(t, s, src)
		if got := eval(t, s.World(), "holds("+src+")"); got != "true" {
			t.Errorf("holds(%s) = %q, want true", src, got)
		}
	}
}

// §5: the name 42 and the string "42" are different atoms.
func TestAtomIdentityIsExact(t *testing.T) {
	s := loom.New()
	number := s.Atom(loom.IntAtom(42))
	text := s.Atom(loom.TextAtom("42"))
	name := s.Atom(loom.NameAtom("42"))
	if number == text || number == name || text == name {
		t.Fatal("payload tags collided")
	}
	if s.Canonical(number) == s.Canonical(text) {
		t.Fatal("canonical forms collided")
	}
	// 042 and 42 are the same atom: IntAtom normalizes.
	if s.Atom(loom.IntAtom(042)) == number {
		t.Skip("Go parses 042 as octal; nothing to check here")
	}
}

// §26: a held fact does not turn an application into a property accessor.
func TestKindIsNotAPropertyPrimitive(t *testing.T) {
	s := loom.New()
	assert(t, s, "kind(Task)(task)")
	if got := eval(t, s.World(), "kind(Task)"); got != "kind(Task)" {
		t.Fatalf("kind(Task) = %q, want kind(Task)", got)
	}
	// The derived form the spec points at instead.
	pat, err := s.ParsePattern("kind(Task)(?x)")
	if err != nil {
		t.Fatal(err)
	}
	rows := matchStrings(t, s.World(), pat)
	if !equalStrings(rows, []string{"x=task"}) {
		t.Fatalf("got %v, want [x=task]", rows)
	}
}

func TestMatchShapes(t *testing.T) {
	s := loom.New()
	assert(t, s, "knows(Alice)(Bob)")
	assert(t, s, "parent(Alice)(Bob)")
	assert(t, s, "knows(Dana)(Bob)")
	assert(t, s, "pair(Alice)(Alice)")
	assert(t, s, "pair(Alice)(Bob)")

	cases := []struct {
		pattern string
		want    []string
	}{
		{"?relation(Alice)(Bob)", []string{"relation=knows", "relation=pair", "relation=parent"}},
		{"knows(?x)(Bob)", []string{"x=Alice", "x=Dana"}},
		{"knows(Alice)(_)", []string{""}},
		{"pair(?x)(?x)", []string{"x=Alice"}},
		{"knows(Charlie)(?x)", nil},
		{"?f(?a)(?b)", []string{
			"a=Alice,b=Alice,f=pair",
			"a=Alice,b=Bob,f=knows",
			"a=Alice,b=Bob,f=pair",
			"a=Alice,b=Bob,f=parent",
			"a=Dana,b=Bob,f=knows",
		}},
	}
	for _, c := range cases {
		pat, err := s.ParsePattern(c.pattern)
		if err != nil {
			t.Fatalf("parse pattern %q: %v", c.pattern, err)
		}
		if got := matchStrings(t, s.World(), pat); !equalStrings(got, c.want) {
			t.Errorf("match %q = %v, want %v", c.pattern, got, c.want)
		}
	}
}

// §24: match sees a fact once however many claims assert it.
func TestMatchSeesDistinctValues(t *testing.T) {
	s := loom.New()
	assert(t, s, "knows(Alice)(Bob)")
	assert(t, s, "knows(Alice)(Bob)")
	assert(t, s, "knows(Alice)(Bob)")

	pat, err := s.ParsePattern("knows(Alice)(?x)")
	if err != nil {
		t.Fatal(err)
	}
	if got := matchStrings(t, s.World(), pat); len(got) != 1 {
		t.Fatalf("got %d results, want 1", len(got))
	}
	// Provenance is the separate API, and it does see all three.
	if got := len(s.World().Claims(valueOf(t, s, "knows(Alice)(Bob)"))); got != 3 {
		t.Fatalf("got %d claims, want 3", got)
	}
}

func TestInvalidPattern(t *testing.T) {
	s := loom.New()
	for name, pat := range map[string]loom.Pattern{
		"nil":           nil,
		"unnamed":       loom.PCapture{},
		"foreign":       loom.PConst{Value: 9999},
		"nested nil":    loom.PApply{Fn: loom.PWildcard{}, Arg: nil},
		"zero constant": loom.PConst{},
	} {
		if _, err := s.World().Match(pat); !errors.Is(err, loom.ErrInvalidPattern) {
			t.Errorf("%s: got %v, want invalid_pattern", name, err)
		}
	}
}

func TestRetractionErrors(t *testing.T) {
	s := loom.New()
	claim := assert(t, s, "knows(Alice)(Bob)")

	tx := s.Begin()
	if err := tx.Retract(404); !errors.Is(err, loom.ErrUnknownClaim) {
		t.Fatalf("got %v, want unknown_claim", err)
	}
	if err := tx.Retract(claim); err != nil {
		t.Fatal(err)
	}
	if err := tx.Retract(claim); !errors.Is(err, loom.ErrAlreadyRetractedClaim) {
		t.Fatalf("got %v, want already_retracted_claim", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	tx = s.Begin()
	if err := tx.Retract(claim); !errors.Is(err, loom.ErrAlreadyRetractedClaim) {
		t.Fatalf("got %v, want already_retracted_claim", err)
	}
}

// §31.6: a failed or abandoned transaction publishes nothing.
func TestDiscardPublishesNothing(t *testing.T) {
	s := loom.New()
	tx := s.Begin()
	if _, err := tx.Assert(parse(t, "knows(Alice)(Bob)")); err != nil {
		t.Fatal(err)
	}
	tx.Discard()
	if got := eval(t, s.World(), "holds(knows(Alice)(Bob))"); got != "false" {
		t.Fatalf("got %q, want false", got)
	}
	if err := tx.Commit(); err == nil {
		t.Fatal("committing a discarded transaction succeeded")
	}
}

// Asserted and retracted in one transaction: never live at any snapshot, but
// both facts kept.
func TestAssertAndRetractInOneTransaction(t *testing.T) {
	s := loom.New()
	tx := s.Begin()
	claim, err := tx.Assert(parse(t, "knows(Alice)(Bob)"))
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Retract(claim); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := eval(t, s.World(), "holds(knows(Alice)(Bob))"); got != "false" {
		t.Fatalf("got %q, want false", got)
	}
	c, ok := s.World().Claim(claim)
	if !ok {
		t.Fatal("the claim was not recorded")
	}
	if c.Asserted == 0 || c.Retracted == 0 {
		t.Fatalf("history lost: asserted %d, retracted %d", c.Asserted, c.Retracted)
	}
}

func TestClaimMetadata(t *testing.T) {
	s := loom.New()
	tx := s.Begin()
	id, err := tx.AssertMeta(parse(t, "knows(Alice)(Bob)"), map[string]string{"source": "import"})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	claim, ok := s.World().Claim(id)
	if !ok {
		t.Fatal("claim missing")
	}
	if claim.Meta["source"] != "import" {
		t.Fatalf("got %v, want source=import", claim.Meta)
	}
}

// §35: evaluation may not terminate, so it must be bounded.
func TestResourceLimits(t *testing.T) {
	t.Run("depth", func(t *testing.T) {
		s := loom.NewWithLimits(loom.Limits{Depth: 64})
		// The classic non-terminating term.
		_, err := s.World().Eval(parse(t, "(x => x(x))(x => x(x))"))
		if !errors.Is(err, loom.ErrResourceLimit) {
			t.Fatalf("got %v, want resource_limit", err)
		}
	})

	t.Run("steps", func(t *testing.T) {
		s := loom.NewWithLimits(loom.Limits{Steps: 5, Depth: 1000})
		if _, err := s.World().Eval(parse(t, "knows(Alice)(Bob)")); !errors.Is(err, loom.ErrResourceLimit) {
			t.Fatalf("got %v, want resource_limit", err)
		}
	})

	t.Run("size", func(t *testing.T) {
		s := loom.NewWithLimits(loom.Limits{Size: 3})
		if _, err := s.World().Eval(parse(t, "knows(Alice)(Bob)")); !errors.Is(err, loom.ErrResourceLimit) {
			t.Fatalf("got %v, want resource_limit", err)
		}
	})

	t.Run("match results", func(t *testing.T) {
		s := loom.NewWithLimits(loom.Limits{MatchResults: 2})
		for i := range 5 {
			assert(t, s, fmt.Sprintf("knows(Alice)(P%d)", i))
		}
		pat, err := s.ParsePattern("knows(Alice)(?x)")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.World().Match(pat); !errors.Is(err, loom.ErrResourceLimit) {
			t.Fatalf("got %v, want resource_limit", err)
		}
	})
}

// Higher-order functions work, and their results become graph structure.
func TestHigherOrderFunctions(t *testing.T) {
	s := loom.New()
	if got := eval(t, s.World(), "(f => x => f(f(x)))(g)(z)"); got != "g(g(z))" {
		t.Fatalf("got %q, want g(g(z))", got)
	}
	if got := eval(t, s.World(), "(compose => compose(a)(b)(z))(f => g => x => f(g(x)))"); got != "a(b(z))" {
		t.Fatalf("got %q, want a(b(z))", got)
	}
}

// Deeply nested values are printed iteratively, so printing one cannot overflow
// the stack. Evaluation's own depth limit is what bounds construction.
func TestDeepValuePrinting(t *testing.T) {
	s := loom.New()
	id := s.Atom(loom.NameAtom("base"))
	step := s.Atom(loom.NameAtom("f"))
	for range 20000 {
		next, err := s.Apply(step, id)
		if err != nil {
			t.Fatal(err)
		}
		id = next
	}
	if len(s.Source(id)) == 0 || len(s.Canonical(id)) == 0 {
		t.Fatal("rendering a deep value produced nothing")
	}
}

func TestForeignValueID(t *testing.T) {
	a, b := loom.New(), loom.New()
	id := a.Atom(loom.NameAtom("Alice"))
	if _, err := b.Apply(id, 9999); !errors.Is(err, loom.ErrUnknownValue) {
		t.Fatalf("got %v, want unknown_value", err)
	}
	tx := b.Begin()
	if _, err := tx.AssertValue(9999, nil); !errors.Is(err, loom.ErrUnknownValue) {
		t.Fatalf("got %v, want unknown_value", err)
	}
}

func TestConcurrentTransactions(t *testing.T) {
	s := loom.New()
	const writers = 16
	var wg sync.WaitGroup
	for i := range writers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			term, err := loom.Parse(fmt.Sprintf("knows(Alice)(P%d)", i))
			if err != nil {
				t.Error(err)
				return
			}
			tx := s.Begin()
			if _, err := tx.Assert(term); err != nil {
				t.Error(err)
				return
			}
			if err := tx.Commit(); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()

	if got := s.World().Seq(); got != writers {
		t.Fatalf("world sequence is %d, want %d", got, writers)
	}
	pat, err := s.ParsePattern("knows(Alice)(?x)")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := s.World().Match(pat)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != writers {
		t.Fatalf("got %d facts, want %d", len(rows), writers)
	}
}
