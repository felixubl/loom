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
//
// The environment travels with the closure. Applying one never resolves its
// free names against the caller's environment, so a function means what it
// meant where it was defined. Closures are not persistable (§13), and v0
// deliberately defines no equality for them (§16).
type Closure struct {
	Param string
	Body  Term
	Env   *Env
}

// Intrinsic is host reduction behavior as a first-class value.
//
// Its identity is the value, not the name it happens to be bound under. The
// base environment binds holds to the holds intrinsic, but a program that
// writes `holds = x => x` rebinds only that name: the intrinsic still exists
// and still means what it meant. Anything compiled, Look included, should hold
// the intrinsic value rather than look up the text of its name.
type Intrinsic struct {
	name string
	prim *Primitive
}

// Name returns the name the base environment binds this intrinsic under. It is
// a label for display, not the intrinsic's identity.
func (i Intrinsic) Name() string { return i.name }

// partial is an intrinsic that has received some but not all of its arguments.
// It exists so an intrinsic of arity greater than one still participates
// through ordinary one-argument application (§11).
type partial struct {
	name string
	prim *Primitive
	args []Value
}

func (Atom) value()      {}
func (Neutral) value()   {}
func (*Closure) value()  {}
func (Intrinsic) value() {}
func (*partial) value()  {}

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
	case Intrinsic:
		return "intrinsic"
	case *partial:
		return "partially applied intrinsic"
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
	case Intrinsic:
		b.WriteString(x.name)
	case *partial:
		b.WriteString(x.name)
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

// Env is a lexical environment: a chain of bindings that a closure captures.
//
// It holds two kinds of binding. A value binding is a lambda parameter or a
// base-environment name, and is already evaluated. A definition binding is a
// cell that exists before its term is evaluated, which is what lets top-level
// definitions be recursive and refer to each other.
type Env struct {
	name   string
	value  Value
	def    *definition
	parent *Env
}

// Bind returns an environment extending e with one value binding. The nil Env
// is the empty environment.
func (e *Env) Bind(name string, v Value) *Env {
	return &Env{name: name, value: v, parent: e}
}

func (e *Env) bindDef(d *definition) *Env {
	return &Env{name: d.name, def: d, parent: e}
}

func (e *Env) find(name string) *Env {
	for ; e != nil; e = e.parent {
		if e.name == name {
			return e
		}
	}
	return nil
}

// Lookup finds the innermost binding of name. A top-level definition that has
// not been evaluated yet is not yet a value, so this reports false for it;
// evaluation forces such a cell instead.
func (e *Env) Lookup(name string) (Value, bool) {
	b := e.find(name)
	if b == nil {
		return nil, false
	}
	if b.def != nil {
		if b.def.state != defReady {
			return nil, false
		}
		return b.def.value, true
	}
	return b.value, true
}

// Names returns every name the environment binds, innermost first, with
// shadowed bindings omitted.
func (e *Env) Names() []string {
	var out []string
	seen := map[string]bool{}
	for ; e != nil; e = e.parent {
		if !seen[e.name] {
			seen[e.name] = true
			out = append(out, e.name)
		}
	}
	return out
}

// Context is what an intrinsic gets: the store it may intern into and the world
// snapshot it may read (§17).
type Context struct {
	Store *Store
	World World
}

// Primitive is host reduction behavior (§11). It is an optimization and a host
// boundary, not an alternate calculus: an intrinsic is applied one argument at
// a time like anything else and only fires once Arity arguments have arrived.
type Primitive struct {
	Arity int
	Apply func(ctx *Context, args []Value) (Value, error)
}

// Register binds an intrinsic into the store's base environment. It replaces
// any binding already made under that name.
//
// Sessions capture the base environment when they are created, so registering
// after a session exists does not reach into it.
func (s *Store) Register(name string, p Primitive) error {
	if p.Arity < 1 {
		return errorf(PrimitiveError, "%s: arity must be at least 1", name)
	}
	if p.Apply == nil {
		return errorf(PrimitiveError, "%s: no implementation", name)
	}
	s.baseMu.Lock()
	defer s.baseMu.Unlock()
	s.base = s.base.Bind(name, Intrinsic{name: name, prim: &p})
	return nil
}

// Base returns the environment holding the intrinsics. It is the root every
// session and every bare evaluation chains onto.
func (s *Store) Base() *Env {
	s.baseMu.RLock()
	defer s.baseMu.RUnlock()
	return s.base
}

// Intrinsic returns the intrinsic value bound under a name. Compiled forms
// should hold this value rather than look the name up later, so that rebinding
// the name cannot change what they mean.
func (s *Store) Intrinsic(name string) (Value, bool) {
	v, ok := s.Base().Lookup(name)
	if !ok {
		return nil, false
	}
	if _, isIntrinsic := v.(Intrinsic); !isIntrinsic {
		return nil, false
	}
	return v, true
}

func registerBuiltins(s *Store) {
	// holds is the only intrinsic v0 ships. It is the world-snapshot layer's
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

// Eval resolves a term against the store's base environment and evaluates it
// there. Every read it performs observes this one snapshot (§17).
//
// No definitions are in scope, so a name the base environment does not bind
// becomes the atom of that name. Use Session to evaluate against a program's
// definitions.
func (w World) Eval(t Term) (Value, error) {
	base := w.store.Base()
	resolved, err := Resolve(t, base)
	if err != nil {
		return nil, err
	}
	return w.EvalIn(resolved, base)
}

// EvalIn evaluates an already-resolved term in an explicit environment. Pass a
// term through Resolve first; an unresolved name here is an error.
func (w World) EvalIn(t Term, env *Env) (Value, error) {
	return w.evaluator().eval(t, env, 0)
}

// Apply applies one runtime value to another against this world snapshot.
func (w World) Apply(fn, arg Value) (Value, error) {
	return w.evaluator().apply(fn, arg, 0)
}

func (w World) evaluator() *evaluator {
	return &evaluator{store: w.store, world: w, limits: w.store.limits}
}

type evaluator struct {
	store  *Store
	world  World
	limits Limits
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
		b := env.find(x.Name)
		if b == nil {
			return nil, errorf(UnboundVariable, "%s", x.Name)
		}
		if b.def != nil {
			return e.force(b.def, depth)
		}
		return b.value, nil

	case TermDef:
		b := env.find(x.Name)
		if b == nil || b.def == nil {
			return nil, errorf(UnboundVariable, "definition %s", x.Name)
		}
		return e.force(b.def, depth)

	case TermName:
		return nil, errorf(UnresolvedName, "%s: pass the term through Resolve first", x.Name)

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

// force evaluates a definition's term the first time anything reads it. A cell
// read while it is already being forced is a definition that depends on its own
// value rather than merely on its own name.
func (e *evaluator) force(d *definition, depth int) (Value, error) {
	switch d.state {
	case defReady:
		return d.value, nil
	case defForcing:
		return nil, errorf(CyclicDefinition, "%s depends on its own value", d.name)
	}
	if d.term == nil {
		return nil, errorf(UnboundVariable, "definition %s has no term", d.name)
	}
	d.state = defForcing
	v, err := e.eval(d.term, d.env, depth+1)
	if err != nil {
		d.state = defPending
		return nil, err
	}
	d.value, d.state = v, defReady
	return v, nil
}

func (e *evaluator) apply(fn, arg Value, depth int) (Value, error) {
	if err := e.step(depth); err != nil {
		return nil, err
	}
	switch f := fn.(type) {
	case *Closure:
		return e.eval(f.Body, f.Env.Bind(f.Param, arg), depth+1)
	case Intrinsic:
		return e.feed(&partial{name: f.name, prim: f.prim}, arg)
	case *partial:
		return e.feed(f, arg)
	}
	return e.neutralize(fn, arg)
}

func (e *evaluator) feed(p *partial, arg Value) (Value, error) {
	args := make([]Value, len(p.args), len(p.args)+1)
	copy(args, p.args)
	args = append(args, arg)
	if len(args) < p.prim.Arity {
		return &partial{name: p.name, prim: p.prim, args: args}, nil
	}
	v, err := p.prim.Apply(&Context{Store: e.store, World: e.world}, args)
	if err != nil {
		var semantic *Error
		if errors.As(err, &semantic) {
			return nil, err
		}
		return nil, errorf(PrimitiveError, "%s: %s", p.name, err)
	}
	if v == nil {
		return nil, errorf(PrimitiveError, "%s returned no value", p.name)
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
