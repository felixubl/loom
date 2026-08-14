package loom

// ClaimID identifies one claim. IDs start at 1.
type ClaimID uint64

// Claim is persistence metadata saying that some source asserted a value (§18).
// The fact and the claim are different things: several claims may assert the
// same value, and the fact holds while any one of them is live.
type Claim struct {
	ID        ClaimID
	Value     ValueID
	Asserted  uint64 // world sequence the claim entered at
	Retracted uint64 // world sequence it left at; zero while live
	Meta      map[string]string
}

// World is an immutable snapshot of the persistent world (§17). Taking one
// fixes the sequence it reads at, so an evaluation never observes a
// transaction that commits after the snapshot was taken.
//
// The zero World is not usable; take one from Store.World.
type World struct {
	store *Store
	seq   uint64
	tx    *Tx // set only inside a transaction, where staged commands are visible (§32)
}

// World returns a snapshot of the current state.
func (s *Store) World() World {
	s.journal.RLock()
	defer s.journal.RUnlock()
	return World{store: s, seq: s.seq}
}

// Seq returns the world sequence this snapshot reads at.
func (w World) Seq() uint64 { return w.seq }

// Store returns the store this snapshot belongs to.
func (w World) Store() *Store { return w.store }

// Holds reports whether at least one live claim asserts this value (§19).
func (w World) Holds(id ValueID) bool {
	if id == 0 {
		return false
	}
	if w.committedHolds(id) {
		return true
	}
	return w.tx != nil && w.tx.holdsStaged(id)
}

func (w World) committedHolds(id ValueID) bool {
	s := w.store
	s.journal.RLock()
	defer s.journal.RUnlock()
	for _, cid := range s.byValue[id] {
		if w.live(&s.claims[cid-1]) {
			return true
		}
	}
	return false
}

// live reports whether a claim is live at this snapshot. Callers hold the
// journal lock.
func (w World) live(c *Claim) bool {
	if c.Asserted == 0 || c.Asserted > w.seq {
		return false
	}
	if c.Retracted != 0 && c.Retracted <= w.seq {
		return false
	}
	return w.tx == nil || !w.tx.retracts(c.ID)
}

// Claim returns one claim as it stands at this snapshot. ok is false for an
// unknown ID or for a claim asserted after the snapshot was taken.
func (w World) Claim(id ClaimID) (Claim, bool) {
	s := w.store
	s.journal.RLock()
	defer s.journal.RUnlock()
	if id == 0 || uint64(id) > uint64(len(s.claims)) {
		return Claim{}, false
	}
	c := s.claims[id-1]
	if c.Asserted == 0 || c.Asserted > w.seq {
		return Claim{}, false
	}
	if c.Retracted > w.seq {
		c.Retracted = 0
	}
	return c, true
}

// Claims returns the live claims asserting a value, in assertion order. This
// is the provenance API: ordinary graph queries ask about facts, not claims
// (§24).
func (w World) Claims(value ValueID) []Claim {
	s := w.store
	s.journal.RLock()
	defer s.journal.RUnlock()
	var out []Claim
	for _, cid := range s.byValue[value] {
		if c := s.claims[cid-1]; w.live(&c) {
			out = append(out, c)
		}
	}
	return out
}

// Values returns the distinct values held at this snapshot, in the order they
// were first asserted. This is what match searches (§24).
func (w World) Values() []ValueID {
	var out []ValueID
	for _, id := range w.candidates() {
		if w.Holds(id) {
			out = append(out, id)
		}
	}
	return out
}

// candidates returns every value that has ever been asserted at or before this
// snapshot, plus anything staged in an open transaction, without deduplicating
// against liveness. Holds decides which of them are actually held.
func (w World) candidates() []ValueID {
	s := w.store
	s.journal.RLock()
	out := make([]ValueID, len(s.asserted))
	copy(out, s.asserted)
	s.journal.RUnlock()
	if w.tx != nil {
		out = append(out, w.tx.stagedValues()...)
	}
	return out
}
