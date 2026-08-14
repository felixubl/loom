package loom

// Tx is a transaction: an ordered sequence of commands (§28). Writes only
// happen here, never during ordinary evaluation, which is what makes them
// impossible to lose, duplicate or reorder (§33).
//
// A Tx, and any World taken from it, must be used from one goroutine. The
// store itself and worlds taken from Store.World are safe to share.
//
// Concurrent transactions in different goroutines commit one at a time, each
// producing its own world sequence. v0 does not detect write skew between them
// beyond refusing to retract a claim another transaction already retracted.
type Tx struct {
	store     *Store
	base      uint64
	cmds      []command
	retracted map[ClaimID]bool
	done      bool
}

type command struct {
	retract bool
	claim   ClaimID
	value   ValueID
	meta    map[string]string
}

// Begin starts a transaction from the current world.
func (s *Store) Begin() *Tx {
	s.journal.RLock()
	defer s.journal.RUnlock()
	return &Tx{store: s, base: s.seq, retracted: map[ClaimID]bool{}}
}

// World returns the transaction's staged world: the world it started from plus
// the commands staged so far (§32). Each command evaluates against this, so a
// later command sees an earlier one's effect while outside readers still see
// the pre-transaction snapshot.
func (tx *Tx) World() World {
	return World{store: tx.store, seq: tx.base, tx: tx}
}

// Assert evaluates a term and stages a claim asserting the resulting value
// (§29). The world holds evaluated values, never unevaluated programs, so
// asserting (x => x)(Alice) asserts Alice.
func (tx *Tx) Assert(t Term) (ClaimID, error) { return tx.AssertMeta(t, nil) }

// AssertMeta is Assert with claim metadata attached, which is how provenance
// gets recorded (§18).
func (tx *Tx) AssertMeta(t Term, meta map[string]string) (ClaimID, error) {
	v, err := tx.World().Eval(t)
	if err != nil {
		return 0, err
	}
	id, err := Persist(v)
	if err != nil {
		return 0, err
	}
	return tx.AssertValue(id, meta)
}

// AssertValue stages a claim asserting an already-evaluated value.
func (tx *Tx) AssertValue(value ValueID, meta map[string]string) (ClaimID, error) {
	if tx.done {
		return 0, errorf(PrimitiveError, "transaction already finished")
	}
	if !tx.store.Has(value) {
		return 0, errorf(UnknownValue, "value %d", value)
	}
	// The id is reserved now rather than at commit so a later command in the
	// same transaction can retract this claim. A discarded transaction just
	// leaves a gap.
	claim := ClaimID(tx.store.reserveClaim())
	tx.cmds = append(tx.cmds, command{claim: claim, value: value, meta: meta})
	return claim, nil
}

// Retract stages the retraction of one exact claim (§30). It does not mean
// "delete every assertion of this value": another claim may assert the same
// value, and that fact keeps holding.
func (tx *Tx) Retract(id ClaimID) error {
	if tx.done {
		return errorf(PrimitiveError, "transaction already finished")
	}
	if tx.retracted[id] {
		return errorf(AlreadyRetractedClaim, "claim %d", id)
	}
	if !tx.staged(id) {
		c, ok := tx.baseClaim(id)
		if !ok {
			return errorf(UnknownClaim, "claim %d", id)
		}
		if c.Retracted != 0 && c.Retracted <= tx.base {
			return errorf(AlreadyRetractedClaim, "claim %d", id)
		}
	}
	tx.retracted[id] = true
	tx.cmds = append(tx.cmds, command{retract: true, claim: id})
	return nil
}

func (tx *Tx) staged(id ClaimID) bool {
	for _, c := range tx.cmds {
		if !c.retract && c.claim == id {
			return true
		}
	}
	return false
}

func (tx *Tx) baseClaim(id ClaimID) (Claim, bool) {
	s := tx.store
	s.journal.RLock()
	defer s.journal.RUnlock()
	if id == 0 || uint64(id) > uint64(len(s.claims)) {
		return Claim{}, false
	}
	c := s.claims[id-1]
	if c.Asserted == 0 || c.Asserted > tx.base {
		return Claim{}, false
	}
	return c, true
}

// Commit publishes every staged command as one new world sequence (§31). Until
// it returns, no outside reader has seen any of them; if it fails, none of them
// is published.
func (tx *Tx) Commit() error {
	if tx.done {
		return errorf(PrimitiveError, "transaction already finished")
	}
	s := tx.store
	s.journal.Lock()
	defer s.journal.Unlock()

	// Re-check retractions under the commit lock. Another transaction may have
	// retracted the same claim since this one staged it, and overwriting its
	// sequence would silently rewrite history.
	for _, c := range tx.cmds {
		if c.retract && uint64(c.claim) <= uint64(len(s.claims)) && s.claims[c.claim-1].Retracted != 0 {
			return errorf(AlreadyRetractedClaim, "claim %d", c.claim)
		}
	}

	seq := s.seq + 1
	for _, c := range tx.cmds {
		if c.retract {
			continue
		}
		s.growClaims(c.claim)
		s.claims[c.claim-1] = Claim{ID: c.claim, Value: c.value, Asserted: seq, Meta: c.meta}
		s.byValue[c.value] = append(s.byValue[c.value], c.claim)
		if !s.assertedI[c.value] {
			s.assertedI[c.value] = true
			s.asserted = append(s.asserted, c.value)
		}
	}
	// Retractions land after the asserts, so a claim staged and retracted in
	// the same transaction exists by the time it is retracted. It then enters
	// and leaves at this same sequence: never live at any snapshot, and both
	// facts kept in the history.
	for _, c := range tx.cmds {
		if c.retract && uint64(c.claim) <= uint64(len(s.claims)) {
			s.claims[c.claim-1].Retracted = seq
		}
	}
	s.seq = seq
	tx.done = true
	return nil
}

// Discard abandons the transaction. Nothing it staged is published.
func (tx *Tx) Discard() {
	tx.done = true
	tx.cmds = nil
}

func (tx *Tx) holdsStaged(value ValueID) bool {
	for _, c := range tx.cmds {
		if !c.retract && c.value == value && !tx.retracted[c.claim] {
			return true
		}
	}
	return false
}

func (tx *Tx) retracts(id ClaimID) bool { return tx.retracted[id] }

func (tx *Tx) stagedValues() []ValueID {
	var out []ValueID
	for _, c := range tx.cmds {
		if !c.retract {
			out = append(out, c.value)
		}
	}
	return out
}

// reserveClaim hands out the next claim id.
func (s *Store) reserveClaim() uint64 {
	s.journal.Lock()
	defer s.journal.Unlock()
	s.nextClaim++
	return s.nextClaim
}

// growClaims makes room for a reserved id. Callers hold the journal lock.
func (s *Store) growClaims(id ClaimID) {
	for uint64(len(s.claims)) < uint64(id) {
		s.claims = append(s.claims, Claim{})
	}
}
