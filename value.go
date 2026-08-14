package loom

import (
	"strconv"
	"strings"
	"sync"
)

// Tag distinguishes atom payloads that would otherwise collide once written
// down: the name 42 and the string "42" are different atoms (§5). It does not
// create different kinds of semantic node — there is exactly one atom node.
type Tag uint8

const (
	TagName Tag = iota
	TagInt
	TagText
)

func (t Tag) String() string {
	switch t {
	case TagName:
		return "name"
	case TagInt:
		return "int"
	case TagText:
		return "text"
	}
	return "tag" + strconv.Itoa(int(t))
}

// Payload is an atom's canonical identity (§5). Two atoms are equal if and only
// if their payloads are equal, so the constructors below normalize: IntAtom
// stores the canonical decimal form, and 042 and 42 are the same atom.
type Payload struct {
	Tag  Tag
	Body string
}

func NameAtom(name string) Payload { return Payload{Tag: TagName, Body: name} }
func TextAtom(text string) Payload { return Payload{Tag: TagText, Body: text} }
func IntAtom(n int64) Payload      { return Payload{Tag: TagInt, Body: strconv.FormatInt(n, 10)} }

// ValueID names a canonical persistable value inside one Store. IDs are dense
// and start at 1; the zero ID is never a value, which is what lets a zero fn
// field mark a node as an atom and a zero Neutral.ID mark an application that
// has no canonical identity.
//
// A ValueID is only meaningful in the Store that issued it.
type ValueID uint32

// node is one entry in the canonical arena. fn == 0 marks an atom.
type node struct {
	payload Payload
	fn, arg ValueID
	size    uint32 // total nodes in this value, for the size limit (§35)
}

type appKey struct{ fn, arg ValueID }

// Store owns the canonical value arena and the claim journal. Everything else
// in the package hangs off one: worlds are snapshots of its journal, and
// ValueIDs index its arena.
//
// A Store is safe for concurrent use.
type Store struct {
	limits Limits

	arena sync.RWMutex
	nodes []node
	atoms map[Payload]ValueID
	apps  map[appKey]ValueID

	prims sync.RWMutex
	prim  map[ValueID]*Primitive

	journal   sync.RWMutex
	claims    []Claim // index = ClaimID - 1
	byValue   map[ValueID][]ClaimID
	asserted  []ValueID // distinct values ever asserted, in first-assert order
	assertedI map[ValueID]bool
	seq       uint64
	nextClaim uint64

	atomTrue, atomFalse ValueID
}

// New returns an empty store with the default limits.
func New() *Store { return NewWithLimits(DefaultLimits()) }

// NewWithLimits returns an empty store with explicit resource limits (§35).
// Limits that are zero or negative fall back to the default for that field.
func NewWithLimits(l Limits) *Store {
	s := &Store{
		limits:    l.withDefaults(),
		atoms:     map[Payload]ValueID{},
		apps:      map[appKey]ValueID{},
		prim:      map[ValueID]*Primitive{},
		byValue:   map[ValueID][]ClaimID{},
		assertedI: map[ValueID]bool{},
	}
	s.atomTrue = s.Atom(NameAtom("true"))
	s.atomFalse = s.Atom(NameAtom("false"))
	registerBuiltins(s)
	return s
}

// Atom interns an atom and returns its canonical identity.
func (s *Store) Atom(p Payload) ValueID {
	s.arena.Lock()
	defer s.arena.Unlock()
	if id, ok := s.atoms[p]; ok {
		return id
	}
	s.nodes = append(s.nodes, node{payload: p, size: 1})
	id := ValueID(len(s.nodes))
	s.atoms[p] = id
	return id
}

// Apply interns the application of one persistable value to another (§37).
// Constructing the same application twice returns the same ValueID, which is
// what turns structural equality into identity equality (§15).
func (s *Store) Apply(fn, arg ValueID) (ValueID, error) {
	s.arena.Lock()
	defer s.arena.Unlock()
	if !s.known(fn) {
		return 0, errorf(UnknownValue, "function %d", fn)
	}
	if !s.known(arg) {
		return 0, errorf(UnknownValue, "argument %d", arg)
	}
	size := s.nodes[fn-1].size + s.nodes[arg-1].size + 1
	if int(size) > s.limits.Size {
		return 0, errorf(ResourceLimit, "value size %d exceeds %d", size, s.limits.Size)
	}
	key := appKey{fn, arg}
	if id, ok := s.apps[key]; ok {
		return id, nil
	}
	s.nodes = append(s.nodes, node{fn: fn, arg: arg, size: size})
	id := ValueID(len(s.nodes))
	s.apps[key] = id
	return id, nil
}

// Bool returns the atom true or the atom false. The kernel has no boolean type
// (§5): these are ordinary name atoms that holds happens to return.
func (s *Store) Bool(b bool) Atom {
	if b {
		return Atom{ID: s.atomTrue}
	}
	return Atom{ID: s.atomFalse}
}

// Payload returns an atom's payload. ok is false when the value is an
// application or the ID is not from this store.
func (s *Store) Payload(id ValueID) (Payload, bool) {
	s.arena.RLock()
	defer s.arena.RUnlock()
	if !s.known(id) || s.nodes[id-1].fn != 0 {
		return Payload{}, false
	}
	return s.nodes[id-1].payload, true
}

// Parts returns an application's function and argument. ok is false when the
// value is an atom or the ID is not from this store.
func (s *Store) Parts(id ValueID) (fn, arg ValueID, ok bool) {
	s.arena.RLock()
	defer s.arena.RUnlock()
	if !s.known(id) || s.nodes[id-1].fn == 0 {
		return 0, 0, false
	}
	n := s.nodes[id-1]
	return n.fn, n.arg, true
}

// known reports whether id indexes a node. Callers hold the arena lock.
func (s *Store) known(id ValueID) bool {
	return id != 0 && int(id) <= len(s.nodes)
}

// Canonical returns the canonical structural representation of a persistable
// value (§36). Two structurally equal values always produce the same string,
// and no two different values produce the same string.
//
//	Atom(name,"Alice")
//	App(App(Atom(name,"knows"),Atom(name,"Alice")),Atom(name,"Bob"))
func (s *Store) Canonical(id ValueID) string {
	return s.render(id, "App(", ",", ")", canonicalAtom)
}

// Source returns the value written in the surface syntax that Parse accepts.
//
//	knows(Alice)(Bob)
func (s *Store) Source(id ValueID) string {
	return s.render(id, "", "(", ")", sourceAtom)
}

func canonicalAtom(b *strings.Builder, p Payload) {
	b.WriteString("Atom(")
	b.WriteString(p.Tag.String())
	b.WriteByte(',')
	b.WriteString(strconv.Quote(p.Body))
	b.WriteByte(')')
}

func sourceAtom(b *strings.Builder, p Payload) {
	if p.Tag == TagText {
		b.WriteString(strconv.Quote(p.Body))
		return
	}
	b.WriteString(p.Body)
}

type renderFrame struct {
	id   ValueID
	step uint8
}

// render walks a value iteratively. Values nest as deeply as evaluation allows,
// so printing must not recurse: a deep value would otherwise overflow the stack
// in the one place that has no resource limit of its own.
func (s *Store) render(id ValueID, open, mid, close string, atom func(*strings.Builder, Payload)) string {
	s.arena.RLock()
	defer s.arena.RUnlock()
	if !s.known(id) {
		return "<unknown>"
	}
	var b strings.Builder
	stack := []renderFrame{{id: id}}
	for len(stack) > 0 {
		top := len(stack) - 1
		f := stack[top]
		n := s.nodes[f.id-1]
		if n.fn == 0 {
			atom(&b, n.payload)
			stack = stack[:top]
			continue
		}
		switch f.step {
		case 0:
			b.WriteString(open)
			stack[top].step = 1
			stack = append(stack, renderFrame{id: n.fn})
		case 1:
			b.WriteString(mid)
			stack[top].step = 2
			stack = append(stack, renderFrame{id: n.arg})
		default:
			b.WriteString(close)
			stack = stack[:top]
		}
	}
	return b.String()
}
