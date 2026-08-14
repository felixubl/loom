package loom

import "sort"

// Definitions binds top-level names to values. A reference to a name it does
// not bind evaluates to the atom of that name instead of failing, which is what
// keeps undefined relations like knows inert.
//
// Definitions are runtime bindings. v0 does not persist them: source-code and
// function persistence are non-goals (§2), and closures are not persistable
// values (§13).
type Definitions struct{ byName map[string]Value }

func NewDefinitions() *Definitions { return &Definitions{byName: map[string]Value{}} }

// Define binds a name, replacing any earlier binding.
func (d *Definitions) Define(name string, v Value) {
	if d != nil {
		d.byName[name] = v
	}
}

// Lookup finds a definition. The nil Definitions binds nothing.
func (d *Definitions) Lookup(name string) (Value, bool) {
	if d == nil {
		return nil, false
	}
	v, ok := d.byName[name]
	return v, ok
}

// Names returns every defined name in sorted order.
func (d *Definitions) Names() []string {
	if d == nil {
		return nil
	}
	out := make([]string, 0, len(d.byName))
	for name := range d.byName {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Command is one statement of a Loom program. Writes are commands rather than
// expressions, which is the whole reason they cannot be lost, duplicated or
// reordered by evaluation (§28, §33).
type Command interface{ command() }

// Define binds a top-level name to the value of a term.
type Define struct {
	Name string
	Term Term
}

// Evaluate is a bare expression. It reads the world but never changes it.
type Evaluate struct{ Term Term }

// Assert evaluates a term and claims the resulting value (§29).
type Assert struct{ Term Term }

// Retract retracts one exact claim (§30).
type Retract struct{ Claim ClaimID }

// Transaction runs assertions and retractions as one atomic step (§31). Only
// Assert and Retract may appear inside one.
type Transaction struct{ Body []Command }

// Query matches a pattern against the values currently held (§21).
type Query struct{ Pattern Pattern }

func (Define) command()      {}
func (Evaluate) command()    {}
func (Assert) command()      {}
func (Retract) command()     {}
func (Transaction) command() {}
func (Query) command()       {}

// Program is a parsed sequence of statements.
type Program struct{ Commands []Command }

// Result is what running one command produced. Which fields are meaningful
// depends on the command: Value for Define and Evaluate, Claims for Assert and
// Transaction, Rows for Query.
type Result struct {
	Command Command
	Name    string
	Value   Value
	Claims  []ClaimID
	Rows    []Bindings
}

// Session is a Loom program in progress: a store plus the definitions made so
// far.
//
// References resolve against the definitions in force when a term is applied,
// not when it was written. That is what lets a definition be recursive, and it
// means a closure taken out of its session and applied through the bare kernel
// API sees no definitions at all.
//
// A Session belongs to one goroutine. The store beneath it may be shared.
type Session struct {
	store *Store
	defs  *Definitions
}

func NewSession(s *Store) *Session {
	return &Session{store: s, defs: NewDefinitions()}
}

func (ss *Session) Store() *Store             { return ss.store }
func (ss *Session) Definitions() *Definitions { return ss.defs }

// Eval evaluates a term against the current world and the session's
// definitions.
func (ss *Session) Eval(t Term) (Value, error) {
	return ss.store.World().EvalWith(t, nil, ss.defs)
}

// Run executes one command.
func (ss *Session) Run(c Command) (Result, error) {
	switch x := c.(type) {
	case Define:
		v, err := ss.Eval(x.Term)
		if err != nil {
			return Result{}, err
		}
		ss.defs.Define(x.Name, v)
		return Result{Command: c, Name: x.Name, Value: v}, nil

	case Evaluate:
		v, err := ss.Eval(x.Term)
		if err != nil {
			return Result{}, err
		}
		return Result{Command: c, Value: v}, nil

	case Assert:
		return ss.atomically(c, []Command{x})

	case Retract:
		return ss.atomically(c, []Command{x})

	case Transaction:
		return ss.atomically(c, x.Body)

	case Query:
		rows, err := ss.store.World().Match(x.Pattern)
		if err != nil {
			return Result{}, err
		}
		return Result{Command: c, Rows: rows}, nil
	}
	return Result{}, errorf(PrimitiveError, "not a command: %T", c)
}

// atomically runs a body of writes as one transaction. Nothing is published
// unless every command in it succeeds (§31).
func (ss *Session) atomically(c Command, body []Command) (Result, error) {
	tx := ss.store.Begin()
	var claims []ClaimID
	for _, inner := range body {
		switch y := inner.(type) {
		case Assert:
			// Evaluated against the staged world, so a later assertion sees an
			// earlier one (§32).
			v, err := tx.World().EvalWith(y.Term, nil, ss.defs)
			if err != nil {
				tx.Discard()
				return Result{}, err
			}
			id, err := Persist(v)
			if err != nil {
				tx.Discard()
				return Result{}, err
			}
			claim, err := tx.AssertValue(id, nil)
			if err != nil {
				tx.Discard()
				return Result{}, err
			}
			claims = append(claims, claim)
		case Retract:
			if err := tx.Retract(y.Claim); err != nil {
				tx.Discard()
				return Result{}, err
			}
		default:
			tx.Discard()
			return Result{}, errorf(PrimitiveError, "only assert and retract may appear in a transaction, found %T", inner)
		}
	}
	if err := tx.Commit(); err != nil {
		return Result{}, err
	}
	return Result{Command: c, Claims: claims}, nil
}

// RunProgram executes every command in order, stopping at the first failure.
func (ss *Session) RunProgram(p *Program) ([]Result, error) {
	out := make([]Result, 0, len(p.Commands))
	for _, c := range p.Commands {
		r, err := ss.Run(c)
		if err != nil {
			return out, err
		}
		out = append(out, r)
	}
	return out, nil
}
