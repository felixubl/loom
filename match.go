package loom

// Pattern is a v0 query pattern (§22). Patterns belong to the persistence and
// query interface, not to the lambda calculus: a capture is not a lambda
// variable and never enters an evaluation environment.
type Pattern interface{ pattern() }

// PConst matches exactly one value.
type PConst struct{ Value ValueID }

// PCapture matches anything and binds it. Repeating a name in one pattern
// requires the two positions to match the same value (§23).
type PCapture struct{ Name string }

// PWildcard matches anything and binds nothing.
type PWildcard struct{}

// PApply matches an application whose function and argument match in turn.
type PApply struct{ Fn, Arg Pattern }

func (PConst) pattern()    {}
func (PCapture) pattern()  {}
func (PWildcard) pattern() {}
func (PApply) pattern()    {}

// Bindings maps capture names to the values they matched (§25).
type Bindings map[string]ValueID

// Match structurally queries the values held at this snapshot (§21). It
// searches distinct held values, so three claims asserting the same value
// produce one result, not three; ask Claims for provenance instead (§24).
//
// The result is a set: ordering carries no meaning in v0. Values are visited in
// the order they were first asserted, which is deterministic within a run but
// is not a canonical cross-host order. A caller that needs one should sort by
// Store.Canonical.
func (w World) Match(p Pattern) ([]Bindings, error) {
	if err := w.store.validPattern(p, 0); err != nil {
		return nil, err
	}
	limit := w.store.limits.MatchResults
	var out []Bindings
	seen := make(map[ValueID]bool)
	for _, id := range w.candidates() {
		if seen[id] {
			continue
		}
		seen[id] = true
		if !w.Holds(id) {
			continue
		}
		b := Bindings{}
		if w.store.matchValue(p, id, b) {
			out = append(out, b)
			if len(out) > limit {
				return nil, errorf(ResourceLimit, "match results exceed %d", limit)
			}
		}
	}
	return out, nil
}

func (s *Store) validPattern(p Pattern, depth int) error {
	if depth > maxNesting {
		return errorf(InvalidPattern, "nested too deeply")
	}
	switch x := p.(type) {
	case PWildcard:
		return nil
	case PCapture:
		if x.Name == "" {
			return errorf(InvalidPattern, "capture with no name")
		}
		return nil
	case PConst:
		if !s.Has(x.Value) {
			return errorf(InvalidPattern, "constant %d is not a value in this store", x.Value)
		}
		return nil
	case PApply:
		if err := s.validPattern(x.Fn, depth+1); err != nil {
			return err
		}
		return s.validPattern(x.Arg, depth+1)
	case nil:
		return errorf(InvalidPattern, "missing pattern")
	}
	return errorf(InvalidPattern, "not a pattern: %T", p)
}

// matchValue matches one value. It only descends as far as the pattern does,
// so its depth is bounded by the pattern the caller wrote, which validPattern
// has already capped.
func (s *Store) matchValue(p Pattern, id ValueID, b Bindings) bool {
	switch x := p.(type) {
	case PWildcard:
		return true
	case PConst:
		return x.Value == id
	case PCapture:
		if prev, ok := b[x.Name]; ok {
			return prev == id
		}
		b[x.Name] = id
		return true
	case PApply:
		fn, arg, ok := s.Parts(id)
		if !ok {
			return false
		}
		return s.matchValue(x.Fn, fn, b) && s.matchValue(x.Arg, arg, b)
	}
	return false
}

// Has reports whether a ValueID names a value in this store.
func (s *Store) Has(id ValueID) bool {
	s.arena.RLock()
	defer s.arena.RUnlock()
	return s.known(id)
}
