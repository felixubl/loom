package loom

import (
	"errors"
	"fmt"
	"strings"
)

// Value is a runtime value. The spec names three (§12) and permits internal
// representations that are observationally equivalent to one of them, which is
// what the unexported partial is.
type Value interface{ value() }

// Atom is an irreducible named or literal value (§5).
type Atom struct{ ID ValueID }

// Neutral is an application whose function had no reduction behavior, so it
// stayed symbolic (§9). This is the bridge between computation and graph
// structure.
//
// ID is the value's canonical identity when both parts are persistable, which
// makes structural equality identity equality (§15). Applying something to a
// closure is still a legal neutral value but has no canonical identity, so
// there ID is zero and only Fn and Arg are meaningful.
type Neutral struct {
	ID      ValueID
	Fn, Arg Value
}

// Closure is a function value over its captured lexical environment (§6).
// Closures are runtime values and are not persistable (§13); v0 deliberately
// defines no equality for them (§16).
type Closure struct {
	Param string
	Body  Term
	Env   *Env
}

// partial is a primitive that has received some but not all of its arguments.
// It exists so a primitive of arity greater than one still participates
// through ordinary one-argument application (§11).
type partial struct {
	atom ValueID
	prim *Primitive
	args []Value
}

func (Atom) value()     {}
func (Neutral) value()  {}
func (*Closure) value() {}
func (*partial) value() {}

// persistID returns the canonical identity of a persistable value, or zero if
// the value is not persistable (§13).
func persistID(v Value) ValueID {
	switch x := v.(type) {
	case Atom:
		return x.ID
	case Neutral:
		return x.ID
	}
	return 0
}

// Persist returns the canonical identity of a value that may be stored, and
// non_persistable_value for one that may not.
func Persist(v Value) (ValueID, error) {
	if id := persistID(v); id != 0 {
		return id, nil
	}
	return 0, errorf(NonPersistableValue, "%s", describe(v))
}

func describe(v Value) string {
	switch v.(type) {
	case *Closure:
		return "closure"
	case *partial:
		return "partially applied primitive"
	case Neutral:
		return "application over a non-persistable value"
	case nil:
		return "nil value"
	}
	return fmt.Sprintf("%T", v)
}

// Display writes any runtime value in surface syntax, including the ones that
// have no canonical identity. Persistable values go through Source and are
// rendered iteratively; closures and applications over them are bounded by the
// evaluation depth limit, so neither can nest without bound.
func (s *Store) Display(v Value) string {
	var b strings.Builder
	s.display(&b, v)
	return b.String()
}

func (s *Store) display(b *strings.Builder, v Value) {
	switch x := v.(type) {
	case Atom:
		b.WriteString(s.Source(x.ID))
	case Neutral:
		if x.ID != 0 {
			b.WriteString(s.Source(x.ID))
			return
		}
		s.display(b, x.Fn)
		b.WriteByte('(')
		s.display(b, x.Arg)
		b.WriteByte(')')
	case *Closure:
		b.WriteString(x.Param)
		b.WriteString(" => ")
		formatTerm(b, x.Body)
	case *partial:
		b.WriteString(s.Source(x.atom))
		for _, arg := range x.args {
			b.WriteByte('(')
			s.display(b, arg)
			b.WriteByte(')')
		}
	case nil:
		b.WriteString("<nil>")
	default:
		fmt.Fprintf(b, "<%T>", v)
	}
}

// Env is a lexical environment: a chain of parameter bindings. Binding by
// environment rather than substitution is what keeps capture correct (§8).
type Env struct {
	name   string
	value  Value
	parent *Env
}

// Bind returns an environment extending e with one binding. The nil Env is the
// empty environment.
func (e *Env) Bind(name string, v Value) *Env {
	return &Env{name: name, value: v, parent: e}
}

// Lookup finds the innermost binding of name.
func (e *Env) Lookup(name string) (Value, bool) {
	for ; e != nil; e = e.parent {
		if e.name == name {
			return e.value, true
		}
	}
	return nil, false
}

// Context is what a primitive gets: the store it may intern into and the world
// snapshot it may read (§17).
type Context struct {
	Store *Store
	World World
}

// Primitive is host reduction behavior attached to an atom (§11). It is an
// optimization and a host boundary, not an alternate calculus: a primitive is
// applied one argument at a time like anything else and only fires once Arity
// arguments have arrived.
type Primitive struct {
	Arity int
	Apply func(ctx *Context, args []Value) (Value, error)
}

// Register attaches reduction behavior to the name atom. It replaces any
// primitive already registered under that name.
func (s *Store) Register(name string, p Primitive) error {
	if p.Arity < 1 {
		return errorf(PrimitiveError, "%s: arity must be at least 1", name)
	}
	if p.Apply == nil {
		return errorf(PrimitiveError, "%s: no implementation", name)
	}
	id := s.Atom(NameAtom(name))
	s.prims.Lock()
	defer s.prims.Unlock()
	s.prim[id] = &p
	return nil
}

func (s *Store) primitive(id ValueID) (*Primitive, bool) {
	s.prims.RLock()
	defer s.prims.RUnlock()
	p, ok := s.prim[id]
	return p, ok
}

func registerBuiltins(s *Store) {
	// holds is the only primitive v0 ships. It is the world-snapshot layer's
	// single read operation (§3, §19); match is a host API instead, because
	// returning a set of bindings needs values v0 does not have yet (§2).
	_ = s.Register("holds", Primitive{Arity: 1, Apply: func(ctx *Context, args []Value) (Value, error) {
		id, err := Persist(args[0])
		if err != nil {
			return nil, err
		}
		return ctx.Store.Bool(ctx.World.Holds(id)), nil
	}})
}

// Eval evaluates a term against this world snapshot. Every read the evaluation
// performs observes this one snapshot (§17).
//
// No definitions are in scope, so every reference falls back to the atom of its
// name. Use Session to evaluate against a program's definitions.
func (w World) Eval(t Term) (Value, error) {
	return w.EvalWith(t, nil, nil)
}

// EvalIn evaluates a term in an explicit environment, which is what a caller
// needs to apply a closure it is holding.
func (w World) EvalIn(t Term, env *Env) (Value, error) {
	return w.EvalWith(t, env, nil)
}

// EvalWith evaluates a term in an explicit environment and against an explicit
// set of top-level definitions. A nil Definitions means none are in scope.
func (w World) EvalWith(t Term, env *Env, defs *Definitions) (Value, error) {
	e := &evaluator{store: w.store, world: w, limits: w.store.limits, defs: defs}
	return e.eval(t, env, 0)
}

// Apply applies one runtime value to another against this world snapshot.
func (w World) Apply(fn, arg Value) (Value, error) {
	e := &evaluator{store: w.store, world: w, limits: w.store.limits}
	return e.apply(fn, arg, 0)
}

type evaluator struct {
	store  *Store
	world  World
	limits Limits
	defs   *Definitions
	steps  int
}

func (e *evaluator) step(depth int) error {
	if depth > e.limits.Depth {
		return errorf(ResourceLimit, "call depth exceeds %d", e.limits.Depth)
	}
	e.steps++
	if e.steps > e.limits.Steps {
		return errorf(ResourceLimit, "evaluation steps exceed %d", e.limits.Steps)
	}
	return nil
}

func (e *evaluator) eval(t Term, env *Env, depth int) (Value, error) {
	if err := e.step(depth); err != nil {
		return nil, err
	}
	switch x := t.(type) {
	case TermAtom:
		return Atom{ID: e.store.Atom(x.Payload)}, nil
	case TermVar:
		v, ok := env.Lookup(x.Name)
		if !ok {
			return nil, errorf(UnboundVariable, "%s", x.Name)
		}
		return v, nil
	case TermRef:
		// A name nothing defines is not an error: it is the atom of that name.
		// This is the kernel staying permissive (§34), and it is what makes
		// knows(Alice)(Bob) graph structure without anyone declaring knows.
		if v, ok := e.defs.Lookup(x.Name); ok {
			return v, nil
		}
		return Atom{ID: e.store.Atom(NameAtom(x.Name))}, nil
	case TermLambda:
		return &Closure{Param: x.Param, Body: x.Body, Env: env}, nil
	case TermApply:
		// Call by value: the function, then the argument, then the application
		// (§7). Evaluating the argument first is what makes
		// knows((x => x)(Alice)) settle to knows(Alice) before it neutralizes.
		fn, err := e.eval(x.Fn, env, depth+1)
		if err != nil {
			return nil, err
		}
		arg, err := e.eval(x.Arg, env, depth+1)
		if err != nil {
			return nil, err
		}
		return e.apply(fn, arg, depth+1)
	}
	return nil, errorf(PrimitiveError, "not a term: %T", t)
}

func (e *evaluator) apply(fn, arg Value, depth int) (Value, error) {
	if err := e.step(depth); err != nil {
		return nil, err
	}
	switch f := fn.(type) {
	case *Closure:
		return e.eval(f.Body, f.Env.Bind(f.Param, arg), depth+1)
	case *partial:
		return e.feed(f, arg)
	case Atom:
		if p, ok := e.store.primitive(f.ID); ok {
			return e.feed(&partial{atom: f.ID, prim: p}, arg)
		}
	}
	return e.neutralize(fn, arg)
}

func (e *evaluator) feed(p *partial, arg Value) (Value, error) {
	args := make([]Value, len(p.args), len(p.args)+1)
	copy(args, p.args)
	args = append(args, arg)
	if len(args) < p.prim.Arity {
		return &partial{atom: p.atom, prim: p.prim, args: args}, nil
	}
	v, err := p.prim.Apply(&Context{Store: e.store, World: e.world}, args)
	if err != nil {
		var semantic *Error
		if errors.As(err, &semantic) {
			return nil, err
		}
		return nil, errorf(PrimitiveError, "%s: %s", e.store.Source(p.atom), err)
	}
	if v == nil {
		return nil, errorf(PrimitiveError, "%s returned no value", e.store.Source(p.atom))
	}
	return v, nil
}

// neutralize builds the symbolic application that an irreducible function
// produces (§9). Application never fails here: a function with no reduction
// behavior yields graph structure, not an error (§34).
func (e *evaluator) neutralize(fn, arg Value) (Value, error) {
	n := Neutral{Fn: fn, Arg: arg}
	f, a := persistID(fn), persistID(arg)
	if f != 0 && a != 0 {
		id, err := e.store.Apply(f, a)
		if err != nil {
			return nil, err
		}
		n.ID = id
	}
	return n, nil
}
