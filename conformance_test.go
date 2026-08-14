// Conformance cases from §40 of docs/loom-v0-spec.md. An implementation is not
// v0-conformant until every one of these passes.
package loom_test

import (
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/felixubl/loom"
)

func parse(t *testing.T, src string) loom.Term {
	t.Helper()
	term, err := loom.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return term
}

// eval evaluates source against a world and returns its surface form, which is
// how the spec writes every expected result.
func eval(t *testing.T, w loom.World, src string) string {
	t.Helper()
	v, err := w.Eval(parse(t, src))
	if err != nil {
		t.Fatalf("eval %q: %v", src, err)
	}
	id, err := loom.Persist(v)
	if err != nil {
		t.Fatalf("persist result of %q: %v", src, err)
	}
	return w.Store().Source(id)
}

// assert runs one assertion in its own transaction and returns the claim id.
func assert(t *testing.T, s *loom.Store, src string) loom.ClaimID {
	t.Helper()
	tx := s.Begin()
	id, err := tx.Assert(parse(t, src))
	if err != nil {
		t.Fatalf("assert %q: %v", src, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit %q: %v", src, err)
	}
	return id
}

func valueOf(t *testing.T, s *loom.Store, src string) loom.ValueID {
	t.Helper()
	v, err := s.World().Eval(parse(t, src))
	if err != nil {
		t.Fatalf("eval %q: %v", src, err)
	}
	id, err := loom.Persist(v)
	if err != nil {
		t.Fatalf("persist %q: %v", src, err)
	}
	return id
}

func Test40_1_Atom(t *testing.T) {
	s := loom.New()
	if got := eval(t, s.World(), "Alice"); got != "Alice" {
		t.Fatalf("got %q, want Alice", got)
	}
}

func Test40_2_Identity(t *testing.T) {
	s := loom.New()
	if got := eval(t, s.World(), "(x => x)(Alice)"); got != "Alice" {
		t.Fatalf("got %q, want Alice", got)
	}
}

func Test40_3_Constant(t *testing.T) {
	s := loom.New()
	if got := eval(t, s.World(), "(x => y => x)(Alice)(Bob)"); got != "Alice" {
		t.Fatalf("got %q, want Alice", got)
	}
}

func Test40_4_NeutralApplication(t *testing.T) {
	s := loom.New()
	if got := eval(t, s.World(), "knows(Alice)"); got != "knows(Alice)" {
		t.Fatalf("got %q, want knows(Alice)", got)
	}
}

func Test40_5_NestedNeutralApplication(t *testing.T) {
	s := loom.New()
	if got := eval(t, s.World(), "knows(Alice)(Bob)"); got != "knows(Alice)(Bob)" {
		t.Fatalf("got %q, want knows(Alice)(Bob)", got)
	}
}

func Test40_6_ArgumentsEvaluateBeforeNeutralization(t *testing.T) {
	s := loom.New()
	if got := eval(t, s.World(), "knows((x => x)(Alice))(Bob)"); got != "knows(Alice)(Bob)" {
		t.Fatalf("got %q, want knows(Alice)(Bob)", got)
	}
}

func Test40_7_StructuralEquality(t *testing.T) {
	s := loom.New()
	a := valueOf(t, s, "knows(Alice)(Bob)")
	b := valueOf(t, s, "knows((x => x)(Alice))((y => y)(Bob))")
	if a != b {
		t.Fatalf("independently constructed values differ: %d vs %d", a, b)
	}
	if s.Canonical(a) != s.Canonical(b) {
		t.Fatalf("canonical forms differ:\n%s\n%s", s.Canonical(a), s.Canonical(b))
	}

	// Structural equality is not accidental interning: a different value must
	// stay different.
	if c := valueOf(t, s, "knows(Bob)(Alice)"); c == a {
		t.Fatal("knows(Bob)(Alice) collided with knows(Alice)(Bob)")
	}
}

func Test40_8_AssertSymbolicValue(t *testing.T) {
	s := loom.New()
	id := assert(t, s, "knows(Alice)(Bob)")
	claim, ok := s.World().Claim(id)
	if !ok {
		t.Fatal("claim not found after commit")
	}
	want := "App(App(Atom(name,\"knows\"),Atom(name,\"Alice\")),Atom(name,\"Bob\"))"
	if got := s.Canonical(claim.Value); got != want {
		t.Fatalf("claim value is %s, want %s", got, want)
	}
}

func Test40_9_Holds(t *testing.T) {
	s := loom.New()
	assert(t, s, "knows(Alice)(Bob)")
	if got := eval(t, s.World(), "holds(knows(Alice)(Bob))"); got != "true" {
		t.Fatalf("got %q, want true", got)
	}
}

func Test40_10_MissingFact(t *testing.T) {
	s := loom.New()
	assert(t, s, "knows(Alice)(Bob)")
	if got := eval(t, s.World(), "holds(knows(Alice)(Charlie))"); got != "false" {
		t.Fatalf("got %q, want false", got)
	}
}

func Test40_11_Match(t *testing.T) {
	s := loom.New()
	assert(t, s, "knows(Alice)(Bob)")
	assert(t, s, "knows(Alice)(Charlie)")
	assert(t, s, "knows(Dana)(Bob)")

	pat, err := s.ParsePattern("knows(Alice)(?x)")
	if err != nil {
		t.Fatalf("parse pattern: %v", err)
	}
	got := matchStrings(t, s.World(), pat)
	want := []string{"x=Bob", "x=Charlie"}
	if !equalStrings(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func Test40_12_DuplicateClaims(t *testing.T) {
	s := loom.New()
	first := assert(t, s, "knows(Alice)(Bob)")
	second := assert(t, s, "knows(Alice)(Bob)")
	if first == second {
		t.Fatal("two assertions of one value produced one claim")
	}

	tx := s.Begin()
	if err := tx.Retract(first); err != nil {
		t.Fatalf("retract first: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if got := eval(t, s.World(), "holds(knows(Alice)(Bob))"); got != "true" {
		t.Fatalf("after retracting one claim: got %q, want true", got)
	}

	tx = s.Begin()
	if err := tx.Retract(second); err != nil {
		t.Fatalf("retract second: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if got := eval(t, s.World(), "holds(knows(Alice)(Bob))"); got != "false" {
		t.Fatalf("after retracting both claims: got %q, want false", got)
	}
}

func Test40_13_ClosurePersistenceRejection(t *testing.T) {
	s := loom.New()
	tx := s.Begin()
	_, err := tx.Assert(parse(t, "x => x"))
	if !errors.Is(err, loom.ErrNonPersistableValue) {
		t.Fatalf("got %v, want non_persistable_value", err)
	}
}

func Test40_14_SnapshotConsistency(t *testing.T) {
	s := loom.New()
	assert(t, s, "knows(Alice)(Bob)")
	before := s.World()

	assert(t, s, "knows(Alice)(Charlie)")

	charlie := valueOf(t, s, "knows(Alice)(Charlie)")
	if before.Holds(charlie) {
		t.Fatal("an old snapshot observed a later transaction")
	}
	if !s.World().Holds(charlie) {
		t.Fatal("a fresh snapshot missed a committed transaction")
	}
	if got := eval(t, before, "holds(knows(Alice)(Charlie))"); got != "false" {
		t.Fatalf("holds against the old snapshot: got %q, want false", got)
	}
}

func Test40_15_TransactionVisibility(t *testing.T) {
	s := loom.New()
	outside := s.World()

	tx := s.Begin()
	if _, err := tx.Assert(parse(t, "knows(Alice)(Bob)")); err != nil {
		t.Fatalf("assert: %v", err)
	}
	if got := eval(t, tx.World(), "holds(knows(Alice)(Bob))"); got != "true" {
		t.Fatalf("staged assertion not visible inside its transaction: got %q", got)
	}
	if got := eval(t, outside, "holds(knows(Alice)(Bob))"); got != "false" {
		t.Fatalf("uncommitted assertion visible outside: got %q", got)
	}

	// The spec's own example: the second command observes the first.
	observed, err := tx.Assert(parse(t, "observed(holds(knows(Alice)(Bob)))"))
	if err != nil {
		t.Fatalf("assert observed: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	claim, ok := s.World().Claim(observed)
	if !ok {
		t.Fatal("observed claim missing after commit")
	}
	if got := s.Source(claim.Value); got != "observed(true)" {
		t.Fatalf("got %q, want observed(true)", got)
	}
}

func matchStrings(t *testing.T, w loom.World, p loom.Pattern) []string {
	t.Helper()
	rows, err := w.Match(p)
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		var parts []string
		for name, id := range row {
			parts = append(parts, name+"="+w.Store().Source(id))
		}
		sort.Strings(parts)
		out = append(out, strings.Join(parts, ","))
	}
	sort.Strings(out)
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
